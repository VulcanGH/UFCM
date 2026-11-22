package notifications

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/notification/services/discord"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Embed struct {
	Enabled bool   `json:"enabled"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Color   int    `json:"color"`
}

type Settings struct {
	Revision   int64                              `json:"revision"`
	Enabled    bool                               `json:"enabled"`
	Configured bool                               `json:"configured"`
	Events     map[domain.NotificationEvent]Embed `json:"events"`
}

type Edit struct {
	Settings
	Webhook *string `json:"webhook"`
}

type Delivery struct {
	ID      string                   `json:"id"`
	Event   domain.NotificationEvent `json:"event"`
	Outcome string                   `json:"outcome"`
	Reason  string                   `json:"reason"`
	Time    string                   `json:"time"`
}

type Repository interface {
	NotificationSettings(context.Context) (Settings, string, error)
	InitNotificationSettings(context.Context, Settings, string) error
	SaveNotificationSettings(context.Context, Settings, string) (Settings, error)
	RecordNotificationDelivery(context.Context, Delivery) error
	NotificationDeliveries(context.Context) ([]Delivery, error)
}

type Service struct {
	repo Repository
	log  zerolog.Logger
	mu   sync.Mutex
}

func Defaults() Settings {
	return Settings{Events: map[domain.NotificationEvent]Embed{
		domain.NotificationEventUFCCopyCompleted:     {true, "UFC Library Copy Completed", "{name}\n{message}", 0x22a447},
		domain.NotificationEventUFCDownloadCompleted: {false, "UFC Download Completed", "{name}\n{message}", 0x22a447},
		domain.NotificationEventUFCAttention:         {false, "UFC Needs Attention", "{name}\n{message}", 0xd43d43},
		domain.NotificationEventAppUpdateAvailable:   {false, "UFC Manager Update Available", "{message}", 0x3498db},
	}}
}

var webhookPath = regexp.MustCompile(`^/api/(v[0-9]+/)?webhooks/[0-9]+/[A-Za-z0-9_-]+/?$`)

func validWebhook(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(u.Hostname() == "discord.com" || u.Hostname() == "discordapp.com") &&
		u.RawQuery == "" && u.Fragment == "" && webhookPath.MatchString(u.EscapedPath())
}

func New(ctx context.Context, repo Repository, legacy []domain.Notification, log zerolog.Logger) (*Service, error) {
	settings := Defaults()
	var candidates []domain.Notification
	for _, entry := range legacy {
		if entry.Type != domain.NotificationTypeDiscord || !entry.Enabled || !validWebhook(entry.Webhook) {
			continue
		}
		for event := range settings.Events {
			if slices.Contains(entry.Events, string(event)) {
				candidates = append(candidates, entry)
				break
			}
		}
	}
	var webhook string
	if len(candidates) == 1 {
		entry := candidates[0]
		webhook, settings.Enabled = entry.Webhook, true
		for event, format := range settings.Events {
			format.Enabled = slices.Contains(entry.Events, string(event))
			settings.Events[event] = format
		}
	}
	if err := repo.InitNotificationSettings(ctx, settings, webhook); err != nil {
		return nil, err
	}
	return &Service{repo: repo, log: log.With().Str("module", "ufc-notifications").Logger()}, nil
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	settings, webhook, err := s.repo.NotificationSettings(ctx)
	settings.Configured = webhook != ""
	return settings, err
}

func (s *Service) Save(ctx context.Context, edit Edit) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defaults := Defaults()
	if len(edit.Events) != len(defaults.Events) {
		return Settings{}, fmt.Errorf("%w: supply a format for each event", catalogue.ErrInvalidEdit)
	}
	for event := range defaults.Events {
		format, ok := edit.Events[event]
		if !ok || strings.TrimSpace(format.Title) == "" || len(format.Title) > 256 ||
			strings.TrimSpace(format.Message) == "" || len(format.Message) > 1500 ||
			format.Color < 0 || format.Color > 0xffffff {
			return Settings{}, fmt.Errorf("%w: invalid embed format", catalogue.ErrInvalidEdit)
		}
	}
	_, webhook, err := s.repo.NotificationSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	if edit.Webhook != nil {
		webhook = strings.TrimSpace(*edit.Webhook)
		if webhook == "" {
			edit.Enabled = false
		}
	}
	if (webhook != "" && !validWebhook(webhook)) || (edit.Enabled && webhook == "") {
		return Settings{}, fmt.Errorf("%w: enter a valid Discord webhook URL", catalogue.ErrInvalidEdit)
	}
	edit.Configured = webhook != ""
	return s.repo.SaveNotificationSettings(ctx, edit.Settings, webhook)
}

func (s *Service) History(ctx context.Context) ([]Delivery, error) {
	return s.repo.NotificationDeliveries(ctx)
}

func (s *Service) Deliver(ctx context.Context, payload domain.NotificationPayload) error {
	return s.deliver(ctx, payload, false)
}

func (s *Service) Skip(ctx context.Context, event domain.NotificationEvent, reason string) {
	s.record(ctx, event, "skipped", reason)
}

func (s *Service) Test(ctx context.Context, event domain.NotificationEvent) error {
	if _, ok := Defaults().Events[event]; !ok {
		return fmt.Errorf("%w: unknown notification event", catalogue.ErrInvalidEdit)
	}
	return s.deliver(ctx, domain.NotificationPayload{
		Event: event, ReleaseName: "UFC Fight Night: Sample Event",
		Message: "This is a UFC Manager test notification.", Timestamp: time.Now(),
	}, true)
}

func (s *Service) deliver(ctx context.Context, payload domain.NotificationPayload, test bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, webhook, err := s.repo.NotificationSettings(ctx)
	if err != nil {
		return err
	}
	format, supported := settings.Events[payload.Event]
	if test && webhook == "" {
		return fmt.Errorf("%w: save a Discord webhook first", catalogue.ErrInvalidEdit)
	}
	if !supported || (!test && (!settings.Enabled || !format.Enabled || webhook == "")) {
		s.record(ctx, payload.Event, "skipped", "Delivery or event disabled")
		return nil
	}
	name := payload.ReleaseName
	if name == "" {
		name = payload.Subject
	}
	render := strings.NewReplacer("{name}", name, "{message}", payload.Message, "{event}", string(payload.Event))
	embed := discord.NewEmbed()
	embed.SetTitle(limit(render.Replace(format.Title), 256))
	embed.SetDescription(limit(render.Replace(format.Message), 4096))
	embed.SetColor(discord.EmbedColors(format.Color))
	embed.SetTimestamp(payload.Timestamp)
	message := discord.NewMessage()
	message.AddEmbed(embed)
	// Use an isolated logger so delivery logs never include message content or credentials.
	sender := discord.NewSender(zerolog.Nop(), discord.Config{WebHookURL: webhook, Name: "UFC Manager"})
	if err := sender.SendMessage(ctx, message); err != nil {
		s.record(ctx, payload.Event, "failed", "Discord did not confirm delivery")
		return fmt.Errorf("Discord did not confirm delivery")
	}
	reason := "Discord confirmed delivery"
	if test {
		reason = "Discord confirmed test delivery"
	}
	s.record(ctx, payload.Event, "sent", reason)
	return nil
}

func limit(value string, length int) string {
	runes := []rune(value)
	return string(runes[:min(len(runes), length)])
}

func (s *Service) record(ctx context.Context, event domain.NotificationEvent, outcome, reason string) {
	s.log.Info().Str("event", string(event)).Str("outcome", outcome).Msg(reason)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.repo.RecordNotificationDelivery(ctx, Delivery{uuid.NewString(), event, outcome, reason, time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		s.log.Warn().Msg("Could not record Discord delivery outcome")
	}
}
