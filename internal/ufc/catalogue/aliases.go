// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"time"
)

// EnrichMissing caches article aliases in small checkpoints during the same refresh.
func (w *Wikipedia) EnrichMissing(ctx context.Context, snapshot *Snapshot, save func(context.Context, *Snapshot) error) error {
	patch := &Snapshot{ListRevision: snapshot.ListRevision, Discovered: snapshot.Discovered, Facts: []Facts{}, Diagnostics: []Diagnostic{}}
	for _, f := range snapshot.Facts {
		if (f.ArticleRevision == 0 || f.forceArticle) && f.articleTarget > 0 {
			patch.AliasTotal++
		}
	}
	if patch.AliasTotal == 0 {
		return nil
	}
	flush := func(saveCtx context.Context) error {
		if len(patch.Facts) == 0 {
			return nil
		}
		if err := save(saveCtx, patch); err != nil {
			return err
		}
		patch.Facts = []Facts{}
		patch.Diagnostics = []Diagnostic{}
		patch.DiagnosticsOmitted = 0
		return nil
	}
	for i, f := range snapshot.Facts {
		if f.ArticleRevision != 0 && !f.forceArticle || f.articleTarget <= 0 {
			continue
		}
		if ctx.Err() != nil {
			checkpoint, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := flush(checkpoint)
			cancel()
			if err != nil {
				return err
			}
			return ctx.Err()
		}
		candidate := f
		candidate.Aliases = append([]Alias{}, f.Aliases...)
		candidate.Review = []string{}
		for _, reason := range f.Review {
			if reason != "article_enrichment_pending" && reason != "article_enrichment_unavailable" {
				addReview(&candidate, reason)
			}
		}
		article, err := w.parse(ctx, f.articleTarget, "")
		if ctx.Err() != nil {
			checkpoint, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			saved := flush(checkpoint)
			cancel()
			if saved != nil {
				return saved
			}
			return ctx.Err()
		}
		if err == nil && article.Parse.PageID == f.PageID && identityTitle(article.Parse.Title) {
			err = enrich(&candidate, article.Parse.Text)
		} else if err == nil {
			addReview(&candidate, "article_identity_conflict")
			err = ErrInvalidEdit
		}
		if err == nil {
			candidate.ArticleRevision = article.Parse.RevID
			candidate.RetrievedAt = time.Now().UTC()
			f = candidate
		} else {
			addReview(&f, "article_enrichment_unavailable")
			addDiagnostic(patch, Diagnostic{Name: f.Name, Title: f.Title, Reason: "article_enrichment_unavailable"})
		}
		snapshot.Facts[i] = f
		patch.Facts = append(patch.Facts, f)
		patch.AliasProcessed++
		if len(patch.Facts) >= 16 {
			if err := flush(ctx); err != nil {
				return err
			}
		}
	}
	return flush(ctx)
}
