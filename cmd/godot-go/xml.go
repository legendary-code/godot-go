package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// classDocXMLVersion is the Godot version stamped into the
// `<class version="...">` attribute of generated docs. The framework's
// floor is 4.4 and the bindings emit against whichever version's
// extension_api.json the user picked, but the docs schema version
// itself has been stable since 4.0; pinning to 4.4 here keeps the
// XML self-describing for the framework's supported surface.
const classDocXMLVersion = "4.4"

// xmlClass is the root <class> element godot's docs system loads via
// editor_help_load_xml_from_utf8_chars. Mirrors the schema documented
// in Godot's doc/class.xsd. Optional attributes use *string so a nil
// pointer omits the attribute entirely (encoding/xml's `,omitempty`
// also drops empty strings, but the *string form is explicit about
// "absent" vs "set-to-empty").
type xmlClass struct {
	XMLName      xml.Name `xml:"class"`
	Name         string   `xml:"name,attr"`
	Inherits     string   `xml:"inherits,attr,omitempty"`
	Version      string   `xml:"version,attr"`
	Deprecated   *string  `xml:"deprecated,attr,omitempty"`
	Experimental *string  `xml:"experimental,attr,omitempty"`

	Brief       string        `xml:"brief_description"`
	Description string        `xml:"description"`
	Tutorials   *xmlTutorials `xml:"tutorials,omitempty"`
	Methods     *xmlMethods   `xml:"methods,omitempty"`
	Members     *xmlMembers   `xml:"members,omitempty"`
	Signals     *xmlSignals   `xml:"signals,omitempty"`
	Constants   *xmlConstants `xml:"constants,omitempty"`
}

type xmlTutorials struct {
	Links []xmlLink `xml:"link"`
}

type xmlLink struct {
	Title string `xml:"title,attr"`
	URL   string `xml:",chardata"`
}

type xmlMethods struct {
	Methods []xmlMethod `xml:"method"`
}

type xmlMethod struct {
	Name         string     `xml:"name,attr"`
	Qualifiers   string     `xml:"qualifiers,attr,omitempty"`
	Deprecated   *string    `xml:"deprecated,attr,omitempty"`
	Experimental *string    `xml:"experimental,attr,omitempty"`
	Return       *xmlReturn `xml:"return,omitempty"`
	Params       []xmlParam `xml:"param"`
	Description  string     `xml:"description"`
}

type xmlReturn struct {
	Type string `xml:"type,attr"`
	Enum string `xml:"enum,attr,omitempty"`
}

type xmlParam struct {
	Index   int    `xml:"index,attr"`
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Enum    string `xml:"enum,attr,omitempty"`
	Default string `xml:"default,attr,omitempty"`
}

type xmlMembers struct {
	Members []xmlMember `xml:"member"`
}

type xmlMember struct {
	Name         string  `xml:"name,attr"`
	Type         string  `xml:"type,attr"`
	Setter       string  `xml:"setter,attr,omitempty"`
	Getter       string  `xml:"getter,attr"`
	Default      string  `xml:"default,attr,omitempty"`
	Enum         string  `xml:"enum,attr,omitempty"`
	Deprecated   *string `xml:"deprecated,attr,omitempty"`
	Experimental *string `xml:"experimental,attr,omitempty"`
	Description  string  `xml:",chardata"`
}

type xmlSignals struct {
	Signals []xmlSignal `xml:"signal"`
}

type xmlSignal struct {
	Name         string     `xml:"name,attr"`
	Deprecated   *string    `xml:"deprecated,attr,omitempty"`
	Experimental *string    `xml:"experimental,attr,omitempty"`
	Params       []xmlParam `xml:"param"`
	Description  string     `xml:"description"`
}

type xmlConstants struct {
	Constants []xmlConstant `xml:"constant"`
}

type xmlConstant struct {
	Name         string  `xml:"name,attr"`
	Value        int64   `xml:"value,attr"`
	Enum         string  `xml:"enum,attr,omitempty"`
	IsBitfield   bool    `xml:"is_bitfield,attr,omitempty"`
	Deprecated   *string `xml:"deprecated,attr,omitempty"`
	Experimental *string `xml:"experimental,attr,omitempty"`
	Description  string  `xml:",chardata"`
}

// composeDescription assembles the visible description text from the
// docInfo's description body plus the @since / @see suffixes. Godot's
// XML schema has no native `since` or `see-also` element, so we render
// these inline using BBCode the editor's docs page formats. Empty
// docInfo returns empty.
func composeDescription(info docInfo) string {
	var b strings.Builder
	body := strings.TrimRight(info.Description, "\n")
	b.WriteString(body)
	if info.Since != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[i]Since: ")
		b.WriteString(info.Since)
		b.WriteString("[/i]")
	}
	if len(info.See) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("See also:")
		for _, s := range info.See {
			b.WriteString(" ")
			b.WriteString(s)
		}
	}
	return b.String()
}

// buildClassXML constructs the <class>…</class> document for one
// extension class and returns the marshaled UTF-8 string the bindings
// pass to editor_help_load_xml_from_utf8_chars at register time.
//
// ec is the per-class emit data — methods, properties, signals,
// enums — already resolved with their docInfo and Godot type names.
// classDocs / classBrief / tutorials carry the class-level
// metadata captured during discovery.
func buildClassXML(ec emitClass, classDocs docInfo, classBrief string, tutorials []tutorialInfo) (string, error) {
	root := xmlClass{
		Name:         ec.Class,
		Inherits:     ec.Parent,
		Version:      classDocXMLVersion,
		Deprecated:   classDocs.Deprecated,
		Experimental: classDocs.Experimental,
		Brief:        classBrief,
		Description:  composeDescription(classDocs),
	}

	if len(tutorials) > 0 {
		t := &xmlTutorials{}
		for _, tu := range tutorials {
			t.Links = append(t.Links, xmlLink{Title: tu.Title, URL: tu.URL})
		}
		root.Tutorials = t
	}

	if len(ec.Methods) > 0 {
		m := &xmlMethods{}
		for _, em := range ec.Methods {
			m.Methods = append(m.Methods, methodToXML(em))
		}
		root.Methods = m
	}

	if len(ec.Properties) > 0 {
		mem := &xmlMembers{}
		for _, ep := range ec.Properties {
			mem.Members = append(mem.Members, propertyToXML(ep))
		}
		root.Members = mem
	}

	if len(ec.Signals) > 0 {
		sig := &xmlSignals{}
		for _, es := range ec.Signals {
			sig.Signals = append(sig.Signals, signalToXML(es))
		}
		root.Signals = sig
	}

	if c := constantsFromEnums(ec.Enums); c != nil {
		root.Constants = c
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "    ")
	if err := enc.Encode(root); err != nil {
		return "", fmt.Errorf("encode class XML: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("flush class XML: %w", err)
	}
	return buf.String(), nil
}

func methodToXML(em emitMethod) xmlMethod {
	out := xmlMethod{
		Name:         em.GodotName,
		Qualifiers:   methodQualifiers(em),
		Deprecated:   em.Deprecated,
		Experimental: em.Experimental,
		Description:  composeDescription(em.docInfo),
	}
	if em.HasReturn {
		out.Return = &xmlReturn{
			Type: em.ReturnGodotType,
			Enum: em.ReturnClassName,
		}
	}
	for i := 0; i < len(em.ArgTypes); i++ {
		name := ""
		if i < len(em.ArgNames) {
			name = em.ArgNames[i]
		}
		gType := ""
		if i < len(em.ArgGodotTypes) {
			gType = em.ArgGodotTypes[i]
		}
		enum := ""
		if i < len(em.ArgClassNames) {
			enum = em.ArgClassNames[i]
		}
		out.Params = append(out.Params, xmlParam{
			Index: i,
			Name:  name,
			Type:  gType,
			Enum:  enum,
		})
	}
	return out
}

// methodQualifiers builds Godot's `qualifiers` attribute (a
// space-separated list — "static", "const", "vararg", etc.) from the
// emitMethod flags. The framework currently only sets static; const
// and vararg surface here when supported.
func methodQualifiers(em emitMethod) string {
	var quals []string
	if em.IsStatic {
		quals = append(quals, "static")
	}
	if em.IsConst {
		quals = append(quals, "const")
	}
	return strings.Join(quals, " ")
}

func propertyToXML(ep emitProperty) xmlMember {
	return xmlMember{
		Name:         ep.GodotName,
		Type:         ep.GodotType,
		Setter:       ep.Setter,
		Getter:       ep.Getter,
		Enum:         ep.ClassName,
		Deprecated:   ep.Deprecated,
		Experimental: ep.Experimental,
		Description:  composeDescription(ep.docInfo),
	}
}

func signalToXML(es emitSignal) xmlSignal {
	out := xmlSignal{
		Name:         es.GodotName,
		Deprecated:   es.Deprecated,
		Experimental: es.Experimental,
		Description:  composeDescription(es.docInfo),
	}
	for i := 0; i < len(es.ArgTypes); i++ {
		name := ""
		if i < len(es.ArgNames) {
			name = es.ArgNames[i]
		}
		gType := ""
		if i < len(es.ArgGodotTypes) {
			gType = es.ArgGodotTypes[i]
		}
		enum := ""
		if i < len(es.ArgClassNames) {
			enum = es.ArgClassNames[i]
		}
		out.Params = append(out.Params, xmlParam{
			Index: i,
			Name:  name,
			Type:  gType,
			Enum:  enum,
		})
	}
	return out
}

// constantsFromEnums flattens emitData.Enums into one <constants> block
// — Godot's docs schema doesn't have a separate `<enums>` container;
// the editor groups by the `enum=` attribute on individual constants.
// Returns nil when there are no values to emit so the encoder skips the
// element entirely.
func constantsFromEnums(enums []emitEnum) *xmlConstants {
	if len(enums) == 0 {
		return nil
	}
	out := &xmlConstants{}
	for _, e := range enums {
		for _, v := range e.Values {
			out.Constants = append(out.Constants, xmlConstant{
				Name:         v.GodotName,
				Value:        v.Value,
				Enum:         e.GoName,
				IsBitfield:   e.IsBitfield,
				Deprecated:   v.Deprecated,
				Experimental: v.Experimental,
				Description:  composeDescription(v.docInfo),
			})
		}
	}
	if len(out.Constants) == 0 {
		return nil
	}
	return out
}
