package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// API is the subset of extension_api.json the bindgen reads. New fields are
// added here as later phases need them; everything we don't decode is
// discarded by encoding/json's default behavior.
type API struct {
	Header                    Header                          `json:"header"`
	BuiltinClassSizes         []BuildConfigSizes              `json:"builtin_class_sizes"`
	BuiltinClassMemberOffsets []BuildConfigMemberOffsets      `json:"builtin_class_member_offsets"`
	BuiltinClasses            []BuiltinClass                  `json:"builtin_classes"`
}

type Header struct {
	VersionMajor    int    `json:"version_major"`
	VersionMinor    int    `json:"version_minor"`
	VersionPatch    int    `json:"version_patch"`
	VersionStatus   string `json:"version_status"`
	VersionFullName string `json:"version_full_name"`
	Precision       string `json:"precision"`
}

type BuildConfigSizes struct {
	BuildConfiguration string      `json:"build_configuration"`
	Sizes              []SizeEntry `json:"sizes"`
}

type SizeEntry struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type BuildConfigMemberOffsets struct {
	BuildConfiguration string                 `json:"build_configuration"`
	Classes            []ClassMemberOffsets   `json:"classes"`
}

type ClassMemberOffsets struct {
	Name    string         `json:"name"`
	Members []MemberOffset `json:"members"`
}

type MemberOffset struct {
	Member string `json:"member"`
	Offset int    `json:"offset"`
	Meta   string `json:"meta"`
}

type BuiltinClass struct {
	Name               string             `json:"name"`
	IndexingReturnType string             `json:"indexing_return_type"`
	IsKeyed            bool               `json:"is_keyed"`
	HasDestructor      bool               `json:"has_destructor"`
	Members            []ClassMember      `json:"members"`
	Constructors       []Constructor      `json:"constructors"`
	Methods            []Method           `json:"methods"`
	Operators          []Operator         `json:"operators"`
}

type ClassMember struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Constructor struct {
	Index     int        `json:"index"`
	Arguments []Argument `json:"arguments"`
}

type Argument struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value"`
}

type Method struct {
	Name       string     `json:"name"`
	ReturnType string     `json:"return_type"`
	IsVararg   bool       `json:"is_vararg"`
	IsConst    bool       `json:"is_const"`
	IsStatic   bool       `json:"is_static"`
	Hash       int64      `json:"hash"`
	Arguments  []Argument `json:"arguments"`
}

type Operator struct {
	Name       string `json:"name"`
	RightType  string `json:"right_type"`
	ReturnType string `json:"return_type"`
}

func loadAPI(path string) (*API, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	api := &API{}
	if err := dec.Decode(api); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return api, nil
}

// SizeFor returns the byte size of a builtin class for the given build config.
func (a *API) SizeFor(buildConfig, className string) (int, bool) {
	for _, bc := range a.BuiltinClassSizes {
		if bc.BuildConfiguration != buildConfig {
			continue
		}
		for _, s := range bc.Sizes {
			if s.Name == className {
				return s.Size, true
			}
		}
	}
	return 0, false
}

// OffsetsFor returns the member offsets for a builtin class under the given
// build config. Returns nil if the class has no offset entries (e.g. types
// like String that aren't directly addressable).
func (a *API) OffsetsFor(buildConfig, className string) []MemberOffset {
	for _, bc := range a.BuiltinClassMemberOffsets {
		if bc.BuildConfiguration != buildConfig {
			continue
		}
		for _, c := range bc.Classes {
			if c.Name == className {
				return c.Members
			}
		}
	}
	return nil
}

// FindBuiltin returns the builtin-class entry by name, or nil if absent.
func (a *API) FindBuiltin(name string) *BuiltinClass {
	for i := range a.BuiltinClasses {
		if a.BuiltinClasses[i].Name == name {
			return &a.BuiltinClasses[i]
		}
	}
	return nil
}
