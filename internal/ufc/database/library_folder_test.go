// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnidentifiedLibraryFolderNamesEventAboveExtras(t *testing.T) {
	for _, relative := range []string{"UFC 315 (2025)/Featurettes/prelims.mkv", "UFC 315 (2025)/Extras/Bonus/prelims.mkv", "UFC 315 (2025)/main.mkv"} {
		require.Equal(t, "UFC 315 (2025)", unidentifiedLibraryFolder(relative, "archive"))
	}
	require.Equal(t, "archive", unidentifiedLibraryFolder("Featurettes/prelims.mkv", "archive"))
}
