package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

func TestFormatSummary_DeterministicSortedAndSectioned(t *testing.T) {
	t.Parallel()

	raw := `Go file: main.go
Package:
  - main

Imports:
  - strings
  - fmt
  - strings

Functions/Methods:
  - zeta
  - alpha
  - alpha

Constants:
  - ZED
  - ALPHA
`

	formattedA := formatSummary(raw, OutputProfileEnhancement)
	formattedB := formatSummary(raw, OutputProfileEnhancement)
	require.Equal(t, formattedA, formattedB)

	require.Contains(t, formattedA, "## Go file: main.go")
	require.Contains(t, formattedA, "### Package")
	require.Contains(t, formattedA, "### Imports")
	require.Contains(t, formattedA, "### Functions/Methods")
	require.Contains(t, formattedA, "### Constants")

	// Sorted and deduplicated disclosure items.
	require.Contains(t, formattedA, "- alpha")
	require.NotContains(t, formattedA, "- alpha\n- alpha")
}

func TestFormatSummary_ProfileSpecificOverflowMarkers(t *testing.T) {
	t.Parallel()

	raw := `TypeScript file: Component.tsx
Functions:
  - one
  - two
  - three
  - four
  - five
  - six
  - seven
  - eight
  - nine
  - ten
`

	enhancement := formatSummary(raw, OutputProfileEnhancement)
	parity := formatSummary(raw, OutputProfileParity)

	require.Contains(t, enhancement, "... and 2 more")
	require.Contains(t, parity, "(+2 more)")
}

func TestFormatSummary_ProfileSpecificRawContentMarkers(t *testing.T) {
	t.Parallel()

	raw := `Text file: notes.txt
Content:
line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12
line 13
line 14
line 15
line 16
line 17
line 18
`

	enhancement := formatSummary(raw, OutputProfileEnhancement)
	parity := formatSummary(raw, OutputProfileParity)

	require.Contains(t, enhancement, "[TRUNCATED] ... and 2 more lines")
	require.Contains(t, parity, "[TRUNCATED] (+2 more lines)")
}

func TestRegistryExplore_UsesFormatterProfile(t *testing.T) {
	t.Parallel()

	content := []byte(`line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12
line 13
line 14
line 15
line 16
line 17
line 18
line 19
line 20
`)

	regEnhancement := NewRegistry(WithOutputProfile(OutputProfileEnhancement))
	regParity := NewRegistry(WithOutputProfile(OutputProfileParity))

	enhancementResult, err := regEnhancement.Explore(context.Background(), ExploreInput{Path: "notes.txt", Content: content})
	require.NoError(t, err)
	parityResult, err := regParity.Explore(context.Background(), ExploreInput{Path: "notes.txt", Content: content})
	require.NoError(t, err)

	require.Contains(t, enhancementResult.Summary, "[TRUNCATED] ... and")
	require.Contains(t, parityResult.Summary, "[TRUNCATED] (+")
}

func TestFormatSummary_GoldenEnhancement(t *testing.T) {
	t.Parallel()
	raw := `Python file: app.py
Imports:
  - import os
  - import sys
  - import pathlib
  - import dataclasses
  - import requests
  - import flask
  - import json
  - import uuid
  - import time

Classes:
  - AuthService(BaseService)
  - UserRepo

Functions:
  - create_app(config)
  - main()
  - helper()
`
	golden.RequireEqual(t, []byte(formatSummary(raw, OutputProfileEnhancement)))
}

func TestFormatSummary_GoldenParity(t *testing.T) {
	t.Parallel()
	raw := `Python file: app.py
Imports:
  - import os
  - import sys
  - import pathlib
  - import dataclasses
  - import requests
  - import flask
  - import json
  - import uuid
  - import time

Classes:
  - AuthService(BaseService)
  - UserRepo

Functions:
  - create_app(config)
  - main()
  - helper()
`
	golden.RequireEqual(t, []byte(formatSummary(raw, OutputProfileParity)))
}

// TestFormatSummary_OverflowMarkerNormalization verifies that overflow markers
// are correctly normalized across parity and enhancement profiles.
func TestFormatSummary_OverflowMarkerNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		raw            string
		profile        OutputProfile
		expectedMarker string
	}{
		{
			name: "enhancement - list overflow",
			raw: `Go file: main.go
Imports:
  - fmt
  - os
  - paths
  - strings
  - time
  - context
  - net
  - http
  - encoding
  - json
`,
			profile:        OutputProfileEnhancement,
			expectedMarker: "- ... and 2 more",
		},
		{
			name: "parity - list overflow",
			raw: `Go file: main.go
Imports:
  - fmt
  - os
  - paths
  - strings
  - time
  - context
  - net
  - http
  - encoding
  - json
`,
			profile:        OutputProfileParity,
			expectedMarker: "- (+2 more)",
		},
		{
			name: "enhancement - content truncation",
			raw: `Text file: notes.txt
Content:
line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12
line 13
line 14
line 15
line 16
line 17
`,
			profile:        OutputProfileEnhancement,
			expectedMarker: "[TRUNCATED] ... and 1 more lines",
		},
		{
			name: "parity - content truncation",
			raw: `Text file: notes.txt
Content:
line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12
line 13
line 14
line 15
line 16
line 17
`,
			profile:        OutputProfileParity,
			expectedMarker: "[TRUNCATED] (+1 more lines)",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			formatted := formatSummary(tt.raw, tt.profile)
			require.Contains(t, formatted, tt.expectedMarker)
		})
	}
}

// TestFormatSummary_MultipleOverflowScenarios tests various overflow scenarios
// across different section types and sizes.
func TestFormatSummary_MultipleOverflowScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantParity  string
		wantEnhance string
	}{
		{
			name: "large imports list",
			raw: `TypeScript file: Component.tsx
Imports:
  - react
  - react-dom
  - @types/node
  - lodash
  - axios
  - moment
  - uuid
  - socket.io-client
  - styled-components
  - react-router-dom
  - material-ui
`,
			wantParity:  "(+3 more)",
			wantEnhance: "... and 3 more",
		},
		{
			name: "many functions",
			raw: `Go file: server.go
Functions:
  - NewServer
  - Start
  - Stop
  - HandleRequest
  - LogRequest
  - HandleError
  - Close
  - Shutdown
  - Restart
  - Configure
`,
			wantParity:  "(+2 more)",
			wantEnhance: "... and 2 more",
		},
		{
			name: "exactly at limit",
			raw: `Python file: test.py
Imports:
  - os
  - sys
  - json
  - time
  - math
  - re
  - typing
  - itertools
`,
			wantParity:  "",
			wantEnhance: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantParity != "" {
				parity := formatSummary(tt.raw, OutputProfileParity)
				require.Contains(t, parity, tt.wantParity)
			}
			if tt.wantEnhance != "" {
				enhance := formatSummary(tt.raw, OutputProfileEnhancement)
				require.Contains(t, enhance, tt.wantEnhance)
			}
		})
	}
}

// TestFormatSummary_Regression_InformationQuality ensures that formatted output
// preserves at least the same amount of structural information as the baseline.
// This test enforces that formatting does not lose key information categories.
func TestFormatSummary_Regression_InformationQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          string
		mustPreserve []string // Must all be present in formatted output
	}{
		{
			name: "python with all categories",
			raw: `Python file: app.py
Imports:
  - os
  - sys
  - json

Classes:
  - AuthService
  - UserRepo

Functions:
  - create_app
  - main
  - handle_request

Constants:
  - MAX_RETRIES
  - TIMEOUT
`,
			mustPreserve: []string{
				"### Imports",
				"### Classes",
				"### Functions",
				"### Constants",
				"AuthService",
				"UserRepo",
				"create_app",
				"main",
				"handle_request",
				"MAX_RETRIES",
				"TIMEOUT",
			},
		},
		{
			name: "mixed bullet styles normalization",
			raw: `Go file: main.go
Package:
  - main

Imports:
  - fmt
  * log
  - strings
  • context

Functions:
  - init
  - main
	- serve
`,
			mustPreserve: []string{
				"### Package",
				"### Imports",
				"### Functions",
				"main",
				"fmt",
				"log",
				"strings",
				"context",
				"init",
				"serve",
			},
		},
		{
			name: "deduplicates entries",
			raw: `Python file: dup.py
Imports:
  - os
  - os
  - json
  - json
  - json

Functions:
  - foo
  - foo
  - bar
`,
			mustPreserve: []string{
				"### Imports",
				"### Functions",
				"os",
				"json",
				"foo",
				"bar",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			formatted := formatSummary(tt.raw, OutputProfileEnhancement)

			for _, must := range tt.mustPreserve {
				require.Contains(t, formatted, must,
					"formatted output must preserve key information")
			}
		})
	}
}

// TestFormatSummary_Regression_SectionOrdering verifies that sections maintain
// their logical ordering and proper markdown header levels.
func TestFormatSummary_Regression_SectionOrdering(t *testing.T) {
	t.Parallel()

	raw := `Python file: app.py
Package:
  - main

Imports:
  - os
  - sys

Classes:
  - Service

Functions:
  - main

Constants:
  - VERSION
`

	formatted := formatSummary(raw, OutputProfileEnhancement)

	// Verify h2 level for file header
	require.Contains(t, formatted, "## Python file: app.py")

	// Verify h3 level for sections
	require.Contains(t, formatted, "### Package")
	require.Contains(t, formatted, "### Imports")
	require.Contains(t, formatted, "### Classes")
	require.Contains(t, formatted, "### Functions")
	require.Contains(t, formatted, "### Constants")

	// Verify sections appear in logical order
	sections := []string{"### Package", "### Imports", "### Classes", "### Functions", "### Constants"}
	lastIdx := -1
	for _, section := range sections {
		idx := strings.Index(formatted, section)
		require.Greater(t, idx, lastIdx,
			"section %s should appear after the previous section", section)
		lastIdx = idx
	}
}
