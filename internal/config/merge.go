package config

import (
	"cmp"
	"maps"
	"slices"
)

func (pc ProviderConfig) merge(t ProviderConfig) ProviderConfig {
	pc.ID = cmp.Or(t.ID, pc.ID)
	pc.Name = cmp.Or(t.Name, pc.Name)
	pc.BaseURL = cmp.Or(t.BaseURL, pc.BaseURL)
	pc.Type = cmp.Or(t.Type, pc.Type)
	pc.APIKey = cmp.Or(t.APIKey, pc.APIKey)
	pc.APIKeyTemplate = cmp.Or(t.APIKeyTemplate, pc.APIKeyTemplate)
	pc.OAuthToken = cmp.Or(t.OAuthToken, pc.OAuthToken)
	pc.Disable = pc.Disable || t.Disable
	pc.SystemPromptPrefix = cmp.Or(t.SystemPromptPrefix, pc.SystemPromptPrefix)
	pc.ExtraHeaders = mergeMaps(pc.ExtraHeaders, t.ExtraHeaders)
	pc.ExtraBody = mergeMaps(pc.ExtraBody, t.ExtraBody)
	pc.ProviderOptions = mergeMaps(pc.ProviderOptions, t.ProviderOptions)
	pc.ExtraParams = mergeMaps(pc.ExtraParams, t.ExtraParams)
	if len(t.Models) > 0 {
		pc.Models = t.Models
	}
	return pc
}

func (m MCPConfig) merge(o MCPConfig) MCPConfig {
	m.Env = mergeMaps(m.Env, o.Env)
	m.Headers = mergeMaps(m.Headers, o.Headers)
	m.Disabled = m.Disabled || o.Disabled
	m.DisabledTools = append(m.DisabledTools, o.DisabledTools...)
	m.Timeout = max(m.Timeout, o.Timeout)
	m.Command = cmp.Or(o.Command, m.Command)
	if len(o.Args) > 0 {
		m.Args = o.Args
	}
	m.Type = cmp.Or(o.Type, m.Type)
	m.URL = cmp.Or(o.URL, m.URL)
	return m
}

func (l LSPConfig) merge(o LSPConfig) LSPConfig {
	l.Env = mergeMaps(l.Env, o.Env)
	l.InitOptions = mergeMaps(l.InitOptions, o.InitOptions)
	l.Options = mergeMaps(l.Options, o.Options)
	l.RootMarkers = sortedCompact(append(l.RootMarkers, o.RootMarkers...))
	l.FileTypes = sortedCompact(append(l.FileTypes, o.FileTypes...))
	l.Disabled = l.Disabled || o.Disabled
	l.Timeout = max(l.Timeout, o.Timeout)
	if len(o.Args) > 0 {
		l.Args = o.Args
	}
	l.Command = cmp.Or(o.Command, l.Command)
	return l
}

func (o TUIOptions) merge(t TUIOptions) TUIOptions {
	o.CompactMode = o.CompactMode || t.CompactMode
	o.DiffMode = cmp.Or(t.DiffMode, o.DiffMode)
	o.Completions.MaxDepth = cmp.Or(t.Completions.MaxDepth, o.Completions.MaxDepth)
	o.Completions.MaxItems = cmp.Or(t.Completions.MaxItems, o.Completions.MaxItems)
	o.Transparent = cmp.Or(t.Transparent, o.Transparent)
	return o
}

func (o Options) merge(t Options) Options {
	o.ContextPaths = append(o.ContextPaths, t.ContextPaths...)
	o.SkillsPaths = append(o.SkillsPaths, t.SkillsPaths...)
	o.Debug = o.Debug || t.Debug
	o.DebugLSP = o.DebugLSP || t.DebugLSP
	o.DisableAutoSummarize = o.DisableAutoSummarize || t.DisableAutoSummarize
	o.DisableProviderAutoUpdate = o.DisableProviderAutoUpdate || t.DisableProviderAutoUpdate
	o.DisableDefaultProviders = o.DisableDefaultProviders || t.DisableDefaultProviders
	o.DisableMetrics = o.DisableMetrics || t.DisableMetrics
	o.DataDirectory = cmp.Or(t.DataDirectory, o.DataDirectory)
	o.InitializeAs = cmp.Or(t.InitializeAs, o.InitializeAs)
	o.DisabledTools = append(o.DisabledTools, t.DisabledTools...)
	o.AutoLSP = cmp.Or(t.AutoLSP, o.AutoLSP)
	o.Progress = cmp.Or(t.Progress, o.Progress)
	if t.TUI != nil {
		if o.TUI == nil {
			o.TUI = &TUIOptions{}
		}
		*o.TUI = o.TUI.merge(*t.TUI)
	}
	if t.Attribution != nil {
		if o.Attribution == nil {
			o.Attribution = &Attribution{}
		}
		o.Attribution.TrailerStyle = cmp.Or(t.Attribution.TrailerStyle, o.Attribution.TrailerStyle)
		o.Attribution.CoAuthoredBy = cmp.Or(t.Attribution.CoAuthoredBy, o.Attribution.CoAuthoredBy)
		o.Attribution.GeneratedWith = o.Attribution.GeneratedWith || t.Attribution.GeneratedWith
	}
	if t.LCM != nil {
		if o.LCM == nil {
			o.LCM = &LCMOptions{}
		}
		o.LCM.CtxCutoffThreshold = cmp.Or(t.LCM.CtxCutoffThreshold, o.LCM.CtxCutoffThreshold)
		o.LCM.DisableLargeToolOutput = o.LCM.DisableLargeToolOutput || t.LCM.DisableLargeToolOutput
		o.LCM.LargeToolOutputTokenThreshold = cmp.Or(t.LCM.LargeToolOutputTokenThreshold, o.LCM.LargeToolOutputTokenThreshold)
		o.LCM.ExplorerOutputProfile = cmp.Or(t.LCM.ExplorerOutputProfile, o.LCM.ExplorerOutputProfile)
	}
	if t.RepoMap != nil {
		if o.RepoMap == nil {
			o.RepoMap = &RepoMapOptions{}
		}
		*o.RepoMap = o.RepoMap.merge(*t.RepoMap)
	}
	return o
}

func (o Tools) merge(t Tools) Tools {
	o.Ls.MaxDepth = cmp.Or(t.Ls.MaxDepth, o.Ls.MaxDepth)
	o.Ls.MaxItems = cmp.Or(t.Ls.MaxItems, o.Ls.MaxItems)
	o.Grep.Timeout = cmp.Or(t.Grep.Timeout, o.Grep.Timeout)
	o.RepoMap = o.RepoMap.merge(t.RepoMap)
	return o
}

func (c Config) merge(t Config) Config {
	for name, mcp := range t.MCP {
		existing, ok := c.MCP[name]
		if !ok {
			c.MCP[name] = mcp
			continue
		}
		c.MCP[name] = existing.merge(mcp)
	}
	for name, lsp := range t.LSP {
		existing, ok := c.LSP[name]
		if !ok {
			c.LSP[name] = lsp
			continue
		}
		c.LSP[name] = existing.merge(lsp)
	}
	// simple override
	maps.Copy(c.Models, t.Models)
	c.Schema = cmp.Or(c.Schema, t.Schema)
	if t.Options != nil {
		*c.Options = c.Options.merge(*t.Options)
	}
	if t.Permissions != nil {
		c.Permissions.AllowedTools = append(c.Permissions.AllowedTools, t.Permissions.AllowedTools...)
	}
	if c.Providers != nil {
		for key, value := range t.Providers.Seq2() {
			existing, ok := c.Providers.Get(key)
			if !ok {
				c.Providers.Set(key, value)
				continue
			}
			c.Providers.Set(key, existing.merge(value))
		}
	}
	c.Tools = c.Tools.merge(t.Tools)

	// RecentModels are not merged - use whichever is not empty
	if len(t.RecentModels) > 0 {
		c.RecentModels = t.RecentModels
	}

	return c
}

func mergeMaps[K comparable, V any](base, overlay map[K]V) map[K]V {
	if base == nil {
		base = make(map[K]V)
	}
	maps.Copy(base, overlay)
	return base
}

func sortedCompact[S ~[]E, E cmp.Ordered](s S) S {
	slices.Sort(s)
	return slices.Compact(s)
}
