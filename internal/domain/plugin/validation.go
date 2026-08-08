package plugin

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed manifest.schema.json
var manifestSchemaJSON []byte

const manifestSchemaURL = "manifest.schema.json"

var (
	schemaOnce sync.Once
	schemaInst *jsonschema.Schema
	schemaErr  error
)

func manifestSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(manifestSchemaJSON)))
		if err != nil {
			schemaErr = err
			return
		}

		c := jsonschema.NewCompiler()
		if err := c.AddResource(manifestSchemaURL, schemaDoc); err != nil {
			schemaErr = err
			return
		}
		schemaInst, schemaErr = c.Compile(manifestSchemaURL)
	})
	return schemaInst, schemaErr
}

// ValidateManifest checks manifest against the JSON Schema contract.
func ValidateManifest(m PluginManifest) error {
	fields, err := validateManifestFields(m)
	if err != nil {
		return err
	}
	if len(fields) > 0 {
		return response.NewValidation(fields)
	}
	return nil
}

func validateManifestFields(m PluginManifest) (map[string]string, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, response.NewInternal("failed to encode manifest").Wrap(err)
	}

	schema, err := manifestSchema()
	if err != nil {
		return nil, response.NewInternal("failed to load manifest schema").Wrap(err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, response.NewInternal("failed to decode manifest").Wrap(err)
	}

	if err := schema.Validate(doc); err != nil {
		return mapValidationError(err), nil
	}
	return nil, nil
}

func mapValidationError(err error) map[string]string {
	fields := map[string]string{}

	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		fields["manifest"] = err.Error()
		return fields
	}

	collectValidationFields(ve, fields)
	if len(fields) == 0 {
		fields["manifest"] = ve.Error()
	}
	return fields
}

func collectValidationFields(ve *jsonschema.ValidationError, fields map[string]string) {
	if len(ve.InstanceLocation) > 0 {
		field := strings.Join(ve.InstanceLocation, ".")
		fields[field] = ve.Error()
	}
	for _, cause := range ve.Causes {
		collectValidationFields(cause, fields)
	}
}

// RedactExecutorConfig returns a copy of the manifest with sensitive executor
// config values removed from client-facing responses.
func RedactExecutorConfig(m PluginManifest) PluginManifest {
	out := m
	if m.Executor.Config == nil {
		return out
	}

	redacted := make(map[string]any, len(m.Executor.Config))
	for k, v := range m.Executor.Config {
		if isSensitiveConfigKey(k) {
			redacted[k] = "[REDACTED]"
			continue
		}
		redacted[k] = v
	}
	out.Executor.Config = redacted
	return out
}

func isSensitiveConfigKey(key string) bool {
	switch strings.ToLower(key) {
	case "api_key", "apikey", "secret", "client_secret", "token",
		"access_token", "refresh_token", "password", "private_key", "credentials":
		return true
	default:
		return false
	}
}
