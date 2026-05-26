package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

func TestLatexCanHandle(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{
			name:     ".tex file",
			path:     "document.tex",
			content:  []byte("\\documentclass{article}"),
			expected: true,
		},
		{
			name:     ".TeX file uppercase",
			path:     "Document.TeX",
			content:  []byte("\\documentclass{article}"),
			expected: true,
		},
		{
			name:     ".latex file",
			path:     "paper.latex",
			content:  []byte("\\documentclass{article}"),
			expected: true,
		},
		{
			name:     ".bst file bibliography style",
			path:     "style.bst",
			content:  []byte("ENTRY { name } { }"),
			expected: true,
		},
		{
			name:     ".json file not LaTeX",
			path:     "config.json",
			content:  []byte(`{"key": "value"}`),
			expected: false,
		},
		{
			name:     ".txt file not LaTeX",
			path:     "readme.txt",
			content:  []byte("just text"),
			expected: false,
		},
	}

	exp := &LatexExplorer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := exp.CanHandle(tt.path, tt.content)
			require.Equal(t, tt.expected, result, "CanHandle()")
		})
	}
}

func TestLatexExtractSections(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectCount  int
		expectLevels map[int]int
		expectTitles []string
	}{
		{
			name: "basic sections",
			content: `\section{Introduction}
\section{Methodology}
\subsection{Data Collection}
\section{Results}`,
			expectCount:  4,
			expectLevels: map[int]int{1: 3, 2: 1},
			expectTitles: []string{"Introduction", "Methodology", "Data Collection", "Results"},
		},
		{
			name: "nested subsections",
			content: `\section{Chapter 1}
\subsection{Section 1.1}
\subsubsection{Section 1.1.1}
\subsection{Section 1.2}
\section{Chapter 2}`,
			expectCount:  5,
			expectLevels: map[int]int{1: 2, 2: 2, 3: 1},
		},
		{
			name: "starred sections",
			content: `\section*{Unnumbered Intro}
\section{Normal Section}`,
			expectCount:  2,
			expectLevels: map[int]int{1: 2},
		},
		{
			name: "paragraph and subparagraph",
			content: `\paragraph{First}
\subparagraph{Sub}
\paragraph{Second}`,
			expectCount:  3,
			expectLevels: map[int]int{4: 2, 5: 1},
		},
		{
			name:         "no sections",
			content:      `Some text without sections`,
			expectCount:  0,
			expectLevels: map[int]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections := extractLatexSections(tt.content)

			require.Equal(t, tt.expectCount, len(sections), "Expected %d sections, got %d", tt.expectCount, len(sections))

			levels := countSectionsByLevel(sections)
			for level, count := range tt.expectLevels {
				require.Equal(t, count, levels[level], "Level %d: expected %d sections, got %d", level, count, levels[level])
			}

			if len(tt.expectTitles) > 0 {
				for i, expectedTitle := range tt.expectTitles {
					if i < len(sections) {
						require.Equal(t, expectedTitle, sections[i].Title, "Section %d: expected title %q, got %q", i, expectedTitle, sections[i].Title)
					}
				}
			}
		})
	}
}

func TestLatexExtractEnvironments(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectEnv     map[string]int
		excludeFilter bool
	}{
		{
			name: "common environments",
			content: `\begin{figure}
\caption{A figure}
\end{figure}

\begin{table}
\end{table}

\begin{equation}
E=mc^2
\end{equation}

\begin{itemize}
\item item 1
\end{itemize}

\begin{enumerate}
\item item 1
\end{enumerate}`,
			expectEnv: map[string]int{
				"figure":    1,
				"table":     1,
				"equation":  1,
				"itemize":   1,
				"enumerate": 1,
			},
		},
		{
			name: "multiple same environments",
			content: `\begin{figure}
\end{figure}
\begin{figure}
\end{figure}
\begin{table}
\end{table}`,
			expectEnv: map[string]int{
				"figure": 2,
				"table":  1,
			},
		},
		{
			name: "filtered environments",
			content: `\begin{document}
\begin{figure}
\end{figure}
\begin{frame}
\end{frame}
\end{document}`,
			expectEnv: map[string]int{
				"figure": 1,
			},
			excludeFilter: false,
		},
		{
			name:      "no environments",
			content:   `Just some text`,
			expectEnv: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs := extractLatexEnvironments(tt.content)

			for name, expectCount := range tt.expectEnv {
				found := false
				for _, env := range envs {
					if env.Name == name {
						found = true
						require.Equal(t, expectCount, env.Count, "Environment %s: expected count %d, got %d", name, expectCount, env.Count)
						break
					}
				}
				require.True(t, found, "Expected environment %s not found", name)
			}
		})
	}
}

func TestLatexExtractBibliography(t *testing.T) {
	tests := []struct {
		name               string
		content            string
		expectBibliography []string
		expectAddbib       []string
		expectCiteCount    int
		expectStyle        string
	}{
		{
			name: "\bibliography command",
			content: `\bibliography{references}
More text
\bibliographystyle{plain}
And \cite{key1} and \citep{key2}.`,
			expectBibliography: []string{"references"},
			expectCiteCount:    2,
			expectStyle:        "plain",
		},
		{
			name: "\addbibresource command",
			content: `\addbibresource{refs.bib}
\cite{author2023}`,
			expectAddbib:    []string{"refs.bib"},
			expectCiteCount: 1,
		},
		{
			name:               "multiple bibliography files",
			content:            `\bibliography{ref1,ref2,ref3}`,
			expectBibliography: []string{"ref1", "ref2", "ref3"},
		},
		{
			name: "various cite commands",
			content: `\cite{key1}
\citep{key2}
\citet{key3}
\citeauthor{key4}
\citeyear{key5}
\nocite{key6}
\cite*[p.45]{key7}`,
			expectCiteCount: 7,
		},
		{
			name:               "no bibliography",
			content:            `Just text`,
			expectBibliography: []string{},
			expectAddbib:       []string{},
			expectCiteCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biblio := extractLatexBibliography(tt.content)

			require.Equal(t, len(tt.expectBibliography), len(biblio.Bibliography), "Expected %d bibliography entries, got %d", len(tt.expectBibliography), len(biblio.Bibliography))
			for i, expected := range tt.expectBibliography {
				if i < len(biblio.Bibliography) {
					require.Equal(t, expected, biblio.Bibliography[i], "Bibliography[%d]: expected %q, got %q", i, expected, biblio.Bibliography[i])
				}
			}

			require.Equal(t, len(tt.expectAddbib), len(biblio.Addbibresource), "Expected %d addbibresource entries, got %d", len(tt.expectAddbib), len(biblio.Addbibresource))
			for i, expected := range tt.expectAddbib {
				if i < len(biblio.Addbibresource) {
					require.Equal(t, expected, biblio.Addbibresource[i], "Addbibresource[%d]: expected %q, got %q", i, expected, biblio.Addbibresource[i])
				}
			}

			require.Equal(t, tt.expectCiteCount, biblio.CiteCount, "Expected %d citations, got %d", tt.expectCiteCount, biblio.CiteCount)

			require.Equal(t, tt.expectStyle, biblio.BibliographySty, "Expected bibliography style %q, got %q", tt.expectStyle, biblio.BibliographySty)
		})
	}
}

func TestLatexExtractPackages(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		expectPkgs []string
	}{
		{
			name: "basic package usage",
			content: `\usepackage{graphicx}
\usepackage{amsmath}
\usepackage{hyperref}`,
			expectPkgs: []string{"graphicx", "amsmath", "hyperref"},
		},
		{
			name: "packages with options",
			content: `\usepackage[utf8]{inputenc}
\usepackage[T1]{fontenc}
\usepackage[colorlinks=true]{hyperref}`,
			expectPkgs: []string{"inputenc", "fontenc", "hyperref"},
		},
		{
			name:       "multiple packages in one command",
			content:    `\usepackage{amsmath,amssymb,amsthm}`,
			expectPkgs: []string{"amsmath", "amssymb", "amsthm"},
		},
		{
			name:       "packages with version",
			content:    `\usepackage{package=v1.0}`,
			expectPkgs: []string{"package"},
		},
		{
			name:       "no packages",
			content:    `Just text`,
			expectPkgs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgs := extractLatexPackages(tt.content)

			require.Equal(t, len(tt.expectPkgs), len(pkgs), "Expected %d packages, got %d: %v", len(tt.expectPkgs), len(pkgs), pkgs)

			for i, expected := range tt.expectPkgs {
				if i < len(pkgs) {
					require.Equal(t, expected, pkgs[i], "Package[%d]: expected %q, got %q", i, expected, pkgs[i])
				}
			}
		})
	}
}

func TestLatexExplore(t *testing.T) {
	exp := &LatexExplorer{}

	t.Run("smoke test basic LaTeX document", func(t *testing.T) {
		content := `\documentclass{article}
\usepackage{graphicx}
\usepackage{amsmath}
\usepackage{hyperref}

\begin{document}

\section{Introduction}
This is the introduction.

\section{Methodology}
\subsection{Data Collection}
We collected data.

\subsection{Analysis}
\begin{equation}
E = mc^2
\end{equation}

\section{Results}
\begin{figure}
\caption{A figure}
\end{figure}

\begin{table}
\end{table}

\begin{itemize}
\item Item 1
\item Item 2
\end{itemize}

\section{Conclusion}

\bibliography{refs}
\bibliographystyle{plain}
Some text with \cite{key1} and \citep{key2}.

\end{document}`

		result, err := exp.Explore(context.Background(), ExploreInput{
			Path:    "paper.tex",
			Content: []byte(content),
		})
		require.NoError(t, err, "Explore failed: %v", err)

		require.Equal(t, "latex", result.ExplorerUsed, "Expected explorer 'latex', got %q", result.ExplorerUsed)

		require.True(t, result.TokenEstimate > 0, "Expected positive token estimate, got %d", result.TokenEstimate)

		// Check for section counts
		expectations := []string{
			"LaTeX file: paper.tex",
			"  - \\section: 4",
			"  - \\subsection: 2",
			"  - figure: 1",
			"  - table: 1",
			"  - equation: 1",
			"  - itemize: 1",
			"Citations: 2",
			"refs",
			"plain",
			"graphicx",
			"amsmath",
			"hyperref",
		}

		for _, exp := range expectations {
			require.True(t, strings.Contains(result.Summary, exp), "Expected summary to contain %q", exp)
		}
	})

	t.Run("\nfile too large", func(t *testing.T) {
		largeContent := make([]byte, MaxFullLoadSize+1)

		result, err := exp.Explore(context.Background(), ExploreInput{
			Path:    "large.tex",
			Content: largeContent,
		})
		require.NoError(t, err, "Explore failed: %v", err)

		require.True(t, strings.Contains(result.Summary, "too large"), "Expected 'too large' in summary for large file")

		require.Equal(t, "latex", result.ExplorerUsed, "Expected explorer 'latex', got %q", result.ExplorerUsed)
	})

	t.Run("BST file handling", func(t *testing.T) {
		content := `ENTRY
{ name }
{ }
{ }`

		result, err := exp.Explore(context.Background(), ExploreInput{
			Path:    "style.bst",
			Content: []byte(content),
		})
		require.NoError(t, err, "Explore failed: %v", err)

		require.Equal(t, "latex", result.ExplorerUsed, "Expected explorer 'latex', got %q", result.ExplorerUsed)
	})

	t.Run("comprehensive bibliography extraction", func(t *testing.T) {
		content := `\documentclass{article}
\begin{document}
\bibliography{ref1,ref2,ref3}
\addbibresource{modern.bib}
\bibliographystyle{abbrv}
See \cite{smith2023}, \citep{jones2024}, and \citet{brown2025}.
Also \citeauthor{doe2023} and \citeyear{doe2023}.
\end{document}`

		result, err := exp.Explore(context.Background(), ExploreInput{
			Path:    "with-bib.tex",
			Content: []byte(content),
		})
		require.NoError(t, err, "Explore failed: %v", err)

		// Check bibliography entries
		require.True(t, strings.Contains(result.Summary, "\\bibliography:"), "Expected to find \\bibliography section")
		for _, ref := range []string{"ref1", "ref2", "ref3"} {
			require.True(t, strings.Contains(result.Summary, ref), "Expected to find bibliography entry %q", ref)
		}

		// Check addbibresource
		require.True(t, strings.Contains(result.Summary, "\\addbibresource:"), "Expected to find \\addbibresource section")
		require.True(t, strings.Contains(result.Summary, "modern.bib"), "Expected to find addbibresource entry 'modern.bib'")

		// Check citation count
		require.True(t, strings.Contains(result.Summary, "Citations: 5"), "Expected 'Citations: 5' in summary")

		// Check style
		require.True(t, strings.Contains(result.Summary, "abbrv"), "Expected to find bibliography style 'abbrv'")
	})
}

func TestLatexEnvironmentsSorting(t *testing.T) {
	content := `\begin{table}
\end{table}
\begin{itemize}
\item a
\end{itemize}
\begin{table}
\end{table}
\begin{enumerate}
\item a
\end{enumerate}`

	envs := extractLatexEnvironments(content)

	// Table should come first (count=2), then enumerate, then itemize (alphabetically for equal counts)
	require.LessOrEqual(t, 2, len(envs), "Expected at least 2 environments, got %d", len(envs))

	require.Equal(t, "table", envs[0].Name, "Expected first env to be table with count 2, got %s with count %d", envs[0].Name, envs[0].Count)
	require.Equal(t, 2, envs[0].Count, "Expected first env to be table with count 2, got %s with count %d", envs[0].Name, envs[0].Count)
}

func TestLatexEmptyContent(t *testing.T) {
	exp := &LatexExplorer{}

	result, err := exp.Explore(context.Background(), ExploreInput{
		Path:    "empty.tex",
		Content: []byte(""),
	})
	require.NoError(t, err, "Explore failed: %v", err)

	require.True(t, strings.Contains(result.Summary, "LaTeX file: empty.tex"), "Expected summary to contain 'LaTeX file: empty.tex'")

	require.True(t, result.TokenEstimate > 0, "Expected positive token estimate for empty file, got %d", result.TokenEstimate)
}

const latexGoldenContent = `\documentclass{article}
\usepackage{graphicx}
\usepackage{amsmath}
\usepackage{amssymb}
\usepackage{hyperref}
\usepackage[utf8]{inputenc}
\usepackage{natbib}
\usepackage{algorithm,algorithm2e}

\begin{document}

\section{Introduction}
This paper presents a novel approach.

\section{Related Work}
\subsection{Background}
Previous work includes \cite{smith2020} and \cite{jones2021}.

\subsection{Limitations}
Current methods have several limitations.

\section{Methodology}
\subsection{Data Collection}
We collected data from multiple sources.

\subsubsection{Preprocessing}
Data preprocessing steps are described here.

\subsection{Model Architecture}
Our model consists of several components.

\section{Experiments}
\subsection{Setup}
Experimental setup parameters.

\section{Results}
\begin{figure}
\caption{Figure showing results}
\end{figure}

\begin{table}
\caption{Table of results}
\end{table}

\begin{equation}
E = mc^2
\end{equation}

\begin{itemize}
\item First item
\item Second item
\end{itemize}

\begin{algorithm}
\caption{Our algorithm}
\end{algorithm}

\section{Conclusion}
We have shown that our approach is effective.

\section{Future Work}
Potential extensions include:

\paragraph{Extension 1}
First extension direction.

\subsection{References}
\bibliography{references,additional}
\bibliographystyle{plainnat}
See also \cite{brown2022}, \citep{wilson2023}, and \citet{davis2024}.

\end{document}`

func TestLatexExplorer_GoldenEnhancement(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(WithOutputProfile(OutputProfileEnhancement))
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "paper.tex",
		Content: []byte(latexGoldenContent),
	})
	require.NoError(t, err)
	golden.RequireEqual(t, []byte(result.Summary))
}

func TestLatexExplorer_GoldenParity(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(WithOutputProfile(OutputProfileParity))
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "paper.tex",
		Content: []byte(latexGoldenContent),
	})
	require.NoError(t, err)
	golden.RequireEqual(t, []byte(result.Summary))
}

// get_string is a helper to safely get a slice element by index.
func get_string(slice []string, idx int) string {
	if idx >= 0 && idx < len(slice) {
		return slice[idx]
	}
	return ""
}
