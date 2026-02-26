# Configuration Development Guide

## Merge Rules

When adding new fields to config structs (`Config`, `Options`, `MCPConfig`, `LSPConfig`, `TUIOptions`, `Tools`, `ProviderConfig`), you **must** update the corresponding `merge()` method in `config.go` and add test cases to `merge_test.go`.

### Merge Behavior Patterns

Each field type has a specific merge strategy:

| Type | Strategy | Example |
|------|----------|---------|
| **Booleans** | `true` if any config has `true` | `Disabled`, `Debug`, `Progress`, `RepoMap.Disabled` |
| **Strings** | Later value replaces earlier | `Model`, `InitializeAs`, `TrailerStyle`, `RepoMap.RefreshMode` |
| **Slices (paths/tools)** | Appended, sorted, deduped | `SkillsPaths`, `DisabledTools`, `RepoMap.ExcludeGlobs` |
| **Slices (args)** | Later replaces earlier entirely | `Args` in LSPConfig |
| **Maps** | Merged, later values overwrite keys | `Env`, `Headers`, `Options` |
| **Timeouts** | Max value wins | `Timeout` in MCPConfig/LSPConfig |
| **Pointers** | Later non-nil replaces earlier | `MaxTokens`, `Temperature`, `Options.RepoMap` |
| **Structs** | Call sub-struct's `merge()` method | `TUI`, `Tools`, `RepoMapOptions` |

### RepoMap Configuration

RepoMap is configured in two places with slightly different types:

- `Options.RepoMap` (`*RepoMapOptions`) — A pointer that gets defaults in `setDefaults()` when nil
- `Tools.RepoMap` (`RepoMapOptions`) — A value type within the `Tools` struct

#### Defaults

When `Options.RepoMap` is nil, `setDefaults()` applies `DefaultRepoMapOptions()`:
```go
func DefaultRepoMapOptions() RepoMapOptions {
    return RepoMapOptions{
        RefreshMode:   "auto",    // Auto-refresh based on changes
        MapMulNoFiles: 2.0,       // Budget multiplier when no files in chat
    }
}
```
Note: `Disabled` defaults to `false` (repo map is enabled by default).

#### RepoMapOptions Merge Rules

`RepoMapOptions.merge()` (from `internal/config/repomap.go`):
- **Disabled**: `OR` — true if either config has `true`
- **MaxTokens**: Last non-zero value (later non-zero overrides earlier)
- **ExcludeGlobs**: Append, then `sortedCompact` (sort + dedup)
- **RefreshMode**: Last non-empty value (later non-empty overrides earlier)
- **MapMulNoFiles**: Last non-zero value (later non-zero overrides earlier)

#### Example

```json
{
    "options": {
        "repo_map": {
            "disabled": false,
            "refresh_mode": "files",
            "map_mul_no_files": 3.0
        }
    },
    "tools": {
        "repo_map": {
            "max_tokens": 4096,
            "exclude_globs": ["vendor/**", "*.log"]
        }
    }
}
```

Both levels merge independently, with `Options.RepoMap` defaulting to `DefaultRepoMapOptions()` and both participating in the full config merge cascade.

### Adding a New Config Field

1. Add the field to the appropriate struct in `config.go`
2. Add merge logic to the struct's `merge()` method following the patterns above
3. Add a test case in `merge_test.go` verifying the merge behavior
4. Run `go test ./internal/config/... -v -run TestConfigMerging` to verify
