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
	Header                    Header                     `json:"header"`
	BuiltinClassSizes         []BuildConfigSizes         `json:"builtin_class_sizes"`
	BuiltinClassMemberOffsets []BuildConfigMemberOffsets `json:"builtin_class_member_offsets"`
	BuiltinClasses            []BuiltinClass             `json:"builtin_classes"`
	Classes                   []Class                    `json:"classes"`
	Singletons                []Singleton                `json:"singletons"`
	UtilityFunctions          []UtilityFunction          `json:"utility_functions"`
	GlobalEnums               []GlobalEnum               `json:"global_enums"`
	NativeStructures          []NativeStructure          `json:"native_structures"`
}

// NativeStructure is a low-level C struct referenced by a handful of
// extension callbacks (physics ray/shape results, text shaping caret info,
// scripting profiling rows). The Format string is C-ish field declarations
// separated by `;`, e.g. "float left;float right".
type NativeStructure struct {
	Name   string `json:"name"`
	Format string `json:"format"`
}

// GlobalEnum is a top-level enum (or bitfield) from extension_api.json#global_enums.
// Names with a dot (e.g. "Variant.Type") collapse to a single Pascal identifier
// in the emitter (VariantType).
type GlobalEnum struct {
	Name       string           `json:"name"`
	IsBitfield bool             `json:"is_bitfield"`
	Values     []GlobalEnumValue `json:"values"`
}

type GlobalEnumValue struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// UtilityFunction is a top-level free function from extension_api.json
// (math, random, general). Bound via variant_get_ptr_utility_function and
// dispatched as a plain ptrcall — no `self` slot.
type UtilityFunction struct {
	Name       string     `json:"name"`
	ReturnType string     `json:"return_type"`
	Category   string     `json:"category"`
	IsVararg   bool       `json:"is_vararg"`
	Hash       int64      `json:"hash"`
	Arguments  []Argument `json:"arguments"`
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

// Class is an engine-class entry from extension_api.json#classes (the host's
// classdb). 944 are tagged api_type="core" and 79 are "editor".
type Class struct {
	Name           string          `json:"name"`
	IsRefcounted   bool            `json:"is_refcounted"`
	IsInstantiable bool            `json:"is_instantiable"`
	Inherits       string          `json:"inherits"`
	APIType        string          `json:"api_type"`
	Constants      []ClassConstant `json:"constants"`
	Enums          []ClassEnum     `json:"enums"`
	Methods        []ClassMethod   `json:"methods"`
	// Signals + properties are decoded but unused in Phase 3b. Properties
	// stay accessible through their underlying Get*/Set* methods; signals
	// arrive in a later phase.
}

type ClassConstant struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type ClassEnum struct {
	Name       string          `json:"name"`
	IsBitfield bool            `json:"is_bitfield"`
	Values     []ClassEnumVal  `json:"values"`
}

type ClassEnumVal struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// ClassMethod is a method on an engine class. Unlike builtin methods, the
// hash here is uint32-range (often above int32 max) and the dispatch path
// goes through classdb_get_method_bind + object_method_bind_ptrcall/_call.
type ClassMethod struct {
	Name       string          `json:"name"`
	IsConst    bool            `json:"is_const"`
	IsVararg   bool            `json:"is_vararg"`
	IsStatic   bool            `json:"is_static"`
	IsVirtual  bool            `json:"is_virtual"`
	IsRequired bool            `json:"is_required"`
	Hash       int64           `json:"hash"`
	ReturnVal  *ClassReturnVal `json:"return_value"`
	Arguments  []ClassArgument `json:"arguments"`
}

type ClassReturnVal struct {
	Type string `json:"type"`
	Meta string `json:"meta"`
}

type ClassArgument struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Meta         string `json:"meta"`
	DefaultValue string `json:"default_value"`
}

// Singleton names a global engine instance (e.g. Engine, Input, OS). The
// Type field gives the class to wrap; Name is what the host registers it as.
type Singleton struct {
	Name string `json:"name"`
	Type string `json:"type"`
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
	// Pre-4.5 JSONs predate the `precision` header field — treat
	// missing as "single" since that was the only option at the time.
	// Patch back into the parsed header so downstream emitters and
	// the build-config check both see the resolved value.
	if api.Header.Precision == "" {
		api.Header.Precision = "single"
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

// FindClass returns the engine-class entry by name, or nil if absent.
func (a *API) FindClass(name string) *Class {
	for i := range a.Classes {
		if a.Classes[i].Name == name {
			return &a.Classes[i]
		}
	}
	return nil
}

// SingletonForType returns the singleton registration for `class`, or nil if
// the class is not a host-registered singleton.
func (a *API) SingletonForType(class string) *Singleton {
	for i := range a.Singletons {
		if a.Singletons[i].Type == class {
			return &a.Singletons[i]
		}
	}
	return nil
}
