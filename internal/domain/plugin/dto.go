package plugin

// RegisterPluginInput is the validated payload for registering a catalog plugin.
type RegisterPluginInput struct {
	Slug        string         `json:"slug" validate:"required,min=2,max=120"`
	Name        string         `json:"name" validate:"required,min=2,max=200"`
	Description string         `json:"description" validate:"max=2000"`
	Version     string         `json:"version" validate:"required,min=1,max=64"`
	Manifest    PluginManifest `json:"manifest" validate:"required"`
}

// UpdatePluginInput is the validated payload for updating a catalog plugin.
type UpdatePluginInput struct {
	Name        string         `json:"name" validate:"required,min=2,max=200"`
	Description string         `json:"description" validate:"max=2000"`
	Version     string         `json:"version" validate:"required,min=1,max=64"`
	Manifest    PluginManifest `json:"manifest" validate:"required"`
}

// ListPluginsInput carries pagination options.
type ListPluginsInput struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// CatalogSummary is the public list representation without full manifest details.
type CatalogSummary struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Version       string    `json:"version"`
	RequiredSetup bool      `json:"required_setup"`
	SetupType     SetupType `json:"setup_type"`
}

// DetailResponse is the public detail representation with a redacted manifest.
type DetailResponse struct {
	ID          string         `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Manifest    PluginManifest `json:"manifest"`
}

// ToCatalogSummary maps a domain entity to its catalog list representation.
func ToCatalogSummary(p *Plugin) CatalogSummary {
	return CatalogSummary{
		ID:            p.ID,
		Slug:          p.Slug,
		Name:          p.Name,
		Description:   p.Description,
		Version:       p.Version,
		RequiredSetup: p.Manifest.RequiredSetup,
		SetupType:     p.Manifest.SetupType,
	}
}

// ToCatalogSummaries maps a slice of entities.
func ToCatalogSummaries(plugins []Plugin) []CatalogSummary {
	out := make([]CatalogSummary, 0, len(plugins))
	for i := range plugins {
		out = append(out, ToCatalogSummary(&plugins[i]))
	}
	return out
}

// ToDetailResponse maps a domain entity to its public detail representation.
func ToDetailResponse(p *Plugin) DetailResponse {
	return DetailResponse{
		ID:          p.ID,
		Slug:        p.Slug,
		Name:        p.Name,
		Description: p.Description,
		Version:     p.Version,
		Manifest:    RedactExecutorConfig(p.Manifest),
	}
}

// PageMeta is pagination metadata returned alongside list responses.
type PageMeta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}
