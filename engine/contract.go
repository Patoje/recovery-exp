package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Contract is the declarative description of one system. Everything specific to the
// system lives here; the engine only knows the closed vocabulary of verifiers.
type Contract struct {
	Version        int                    `yaml:"version"`
	Name           string                 `yaml:"name"`
	Environment    Environment            `yaml:"environment"`
	ObjectivesFile string                 `yaml:"objectives_file"`
	Components     []Component            `yaml:"components"`
	WitnessTypes   map[string]WitnessType `yaml:"witness_types"`
	Checks         []Check                `yaml:"checks"`
}

type Environment struct {
	ComposeFile string `yaml:"compose_file"`
	EnvFile     string `yaml:"env_file"`
	Probe       Probe  `yaml:"probe"`
}

// Probe is the service started inside the restored environment's network to run
// network operations (HTTP, Git). The engine mounts its own binary into it.
type Probe struct {
	Service  string   `yaml:"service"`
	Profiles []string `yaml:"profiles"`
}

// Component is a service of the restored system. The list order is the expected start order.
type Component struct {
	Name    string `yaml:"name"`
	Service string `yaml:"service"`
}

// WitnessType says how to tell whether one witness of that type was recovered.
type WitnessType struct {
	Request HTTPSpec   `yaml:"request"`
	Expect  ExpectSpec `yaml:"expect"`
}

type HTTPSpec struct {
	Method string `yaml:"method"`
	URL    string `yaml:"url"`
	Auth   *Auth  `yaml:"auth"`
	// FollowJSONField: after a 2xx JSON response, GET the URL found in this field (same auth).
	FollowJSONField string `yaml:"follow_json_field"`
}

type Auth struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type ExpectSpec struct {
	Status int `yaml:"status"`
	// JSON: top-level fields of the (final) JSON body and their expected values.
	JSON map[string]string `yaml:"json"`
	// SHA256OfBody: expected sha256 (hex) of the final body.
	SHA256OfBody string `yaml:"sha256_of_body"`
}

type Check struct {
	ID       string    `yaml:"id"`
	Verifier string    `yaml:"verifier"`
	With     yaml.Node `yaml:"with"`
	params   any
}

var idRx = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// LoadContract parses and validates a contract. Unknown fields, unknown verifiers and
// unknown parameters are errors.
func LoadContract(path string) (*Contract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	c, err := ParseContract(raw)
	return c, raw, err
}

func ParseContract(raw []byte) (*Contract, error) {
	var c Contract
	if err := strictDecode(raw, &c); err != nil {
		return nil, fmt.Errorf("contrato: %w", err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("contrato: version %d no soportada (solo 1)", c.Version)
	}
	if c.Environment.ComposeFile == "" || c.Environment.Probe.Service == "" {
		return nil, fmt.Errorf("contrato: environment.compose_file y environment.probe.service son obligatorios")
	}
	comps := map[string]bool{}
	for _, comp := range c.Components {
		if comp.Name == "" || comp.Service == "" {
			return nil, fmt.Errorf("contrato: componente sin name o service")
		}
		comps[comp.Name] = true
	}
	ids := map[string]bool{}
	for i := range c.Checks {
		ch := &c.Checks[i]
		if !idRx.MatchString(ch.ID) || ids[ch.ID] {
			return nil, fmt.Errorf("contrato: id de verificación inválido o repetido: %q", ch.ID)
		}
		ids[ch.ID] = true
		v, ok := verifiers[ch.Verifier]
		if !ok {
			return nil, fmt.Errorf("contrato: %s: verificador desconocido %q", ch.ID, ch.Verifier)
		}
		p := v.newParams()
		if ch.With.Kind != 0 {
			b, err := yaml.Marshal(&ch.With)
			if err != nil {
				return nil, err
			}
			if err := strictDecode(b, p); err != nil {
				return nil, fmt.Errorf("contrato: %s (%s): %w", ch.ID, ch.Verifier, err)
			}
		}
		if err := v.validate(&c, p, comps); err != nil {
			return nil, fmt.Errorf("contrato: %s (%s): %w", ch.ID, ch.Verifier, err)
		}
		ch.params = p
	}
	for i := range c.Checks {
		if r, ok := c.Checks[i].params.(*RTOParams); ok {
			for _, id := range r.Requires {
				if !ids[id] {
					return nil, fmt.Errorf("contrato: %s: requires menciona %q, que no existe", c.Checks[i].ID, id)
				}
			}
		}
	}
	return &c, nil
}

func strictDecode(raw []byte, out any) error {
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	return d.Decode(out)
}

func (c *Contract) component(name string) (Component, bool) {
	for _, comp := range c.Components {
		if comp.Name == name {
			return comp, true
		}
	}
	return Component{}, false
}
