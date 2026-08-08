package userplugin

import (
	"github.com/cymonevo/go_template/internal/domain/plugin"
)

// InstallInput is the validated payload for installing a plugin.
type InstallInput struct {
	PluginSlug string `json:"plugin_slug" validate:"required"`
}

// SetEnabledInput is the validated payload for toggling plugin enablement.
type SetEnabledInput struct {
	Enabled bool `json:"enabled"`
}

// PluginSummary is the catalog fields included in installed-plugin responses.
type PluginSummary struct {
	ID            string           `json:"id"`
	Slug          string           `json:"slug"`
	Name          string           `json:"name"`
	RequiredSetup bool             `json:"required_setup"`
	SetupType     plugin.SetupType `json:"setup_type"`
}

// InstalledResponse is the public representation of an installed plugin.
type InstalledResponse struct {
	ID          string        `json:"id"`
	Enabled     bool          `json:"enabled"`
	SetupStatus SetupStatus   `json:"setup_status"`
	Plugin      PluginSummary `json:"plugin"`
}

// ToInstalledResponse maps a user plugin and catalog entry to the API response.
func ToInstalledResponse(up *UserPlugin, p *plugin.Plugin) InstalledResponse {
	return InstalledResponse{
		ID:          up.ID,
		Enabled:     up.Enabled,
		SetupStatus: up.SetupStatus,
		Plugin: PluginSummary{
			ID:            p.ID,
			Slug:          p.Slug,
			Name:          p.Name,
			RequiredSetup: p.Manifest.RequiredSetup,
			SetupType:     p.Manifest.SetupType,
		},
	}
}

// ToInstalledResponses maps multiple installs using a plugin lookup table.
func ToInstalledResponses(installs []UserPlugin, plugins map[string]*plugin.Plugin) []InstalledResponse {
	out := make([]InstalledResponse, 0, len(installs))
	for i := range installs {
		p, ok := plugins[installs[i].PluginID]
		if !ok {
			continue
		}
		out = append(out, ToInstalledResponse(&installs[i], p))
	}
	return out
}
