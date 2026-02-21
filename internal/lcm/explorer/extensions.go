package explorer

// TEXT_EXTENSIONS is the set of file extensions treated as text.
var TEXT_EXTENSIONS = map[string]string{
	// Go
	"go": "go",
	// Python
	"py": "python", "pyw": "python", "pyx": "python", "pxd": "python",
	// JavaScript
	"js": "javascript", "mjs": "javascript", "cjs": "javascript",
	// TypeScript
	"ts": "typescript", "tsx": "typescript", "mts": "typescript", "cts": "typescript",
	// JSX
	"jsx": "javascript",
	// Rust
	"rs": "rust",
	// Java
	"java": "java",
	// C/C++
	"c": "c", "h": "c",
	"cpp": "cpp", "cxx": "cpp", "cc": "cpp", "hpp": "cpp", "hxx": "cpp", "hh": "cpp",
	// C#
	"cs": "csharp",
	// Ruby
	"rb": "ruby", "rake": "ruby",
	// Shell
	"sh": "shell", "bash": "shell", "zsh": "shell", "fish": "shell",
	"ksh": "shell", "csh": "shell", "tcsh": "shell",
	// PHP
	"php": "php",
	// Kotlin
	"kt": "kotlin", "kts": "kotlin",
	// Scala
	"scala": "scala", "sc": "scala",
	// Swift
	"swift": "swift",
	// SQL
	"sql": "sql",
	// HTML/CSS
	"html": "html", "htm": "html", "xhtml": "html",
	"css": "css", "scss": "scss", "sass": "sass", "less": "less",
	// Data formats
	"json": "json", "jsonc": "json", "json5": "json",
	"yaml": "yaml", "yml": "yaml",
	"toml": "toml",
	"xml":  "xml", "xsd": "xml", "xsl": "xml", "xslt": "xml",
	"csv": "csv", "tsv": "csv",
	"ini": "ini", "cfg": "ini", "conf": "ini", "config": "ini", "properties": "ini",
	// Markdown/docs
	"md": "markdown", "markdown": "markdown",
	"rst":  "rst",
	"txt":  "text",
	"adoc": "asciidoc",
	// LaTeX
	"tex": "latex", "latex": "latex", "bst": "latex",
	// Log
	"log": "text", "stderr": "text", "stdout": "text",
	// Web
	"vue": "javascript", "svelte": "javascript",
	// Elixir
	"ex": "elixir", "exs": "elixir",
	// Erlang
	"erl": "erlang", "hrl": "erlang",
	// Haskell
	"hs": "haskell", "lhs": "haskell",
	// OCaml
	"ml": "ocaml", "mli": "ocaml",
	// F#
	"fs": "fsharp", "fsx": "fsharp",
	// Lua
	"lua": "lua",
	// R
	"r": "r", "R": "r",
	// Julia
	"jl": "julia",
	// Perl
	"pl": "perl", "pm": "perl",
	// Groovy
	"groovy": "groovy",
	// Makefile
	"makefile": "makefile", "mk": "makefile",
	// WASM
	"wat": "wasm",
	// Proto
	"proto": "protobuf",
	// Terraform
	"tf": "terraform", "tfvars": "terraform",
	// Dockerfile
	"dockerfile": "dockerfile",
}

// BINARY_EXTENSIONS is the set of file extensions always treated as binary.
var BINARY_EXTENSIONS = map[string]struct{}{
	"exe": {}, "dll": {}, "so": {}, "dylib": {}, "a": {}, "lib": {},
	"obj": {}, "bin": {}, "elf": {},
	"app": {}, "deb": {}, "rpm": {}, "msi": {}, "dmg": {},
	"png": {}, "jpg": {}, "jpeg": {}, "gif": {}, "bmp": {}, "ico": {}, "svg": {},
	"webp": {}, "tiff": {}, "tif": {},
	"raw": {}, "cr2": {}, "nef": {}, "arw": {}, "dng": {}, "psd": {},
	"mp3": {}, "mp4": {}, "avi": {}, "mov": {}, "mkv": {}, "wav": {}, "flac": {},
	"zip": {}, "tar": {}, "gz": {}, "bz2": {}, "xz": {}, "7z": {}, "rar": {},
	"pdf":   {},
	"class": {}, "jar": {},
	"wasm": {},
	"pyc":  {}, "pyo": {},
	"o": {},
}
