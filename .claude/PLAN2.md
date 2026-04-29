Background
==========
Up to this point we've created, we've created an easy to use framework for using idiomatic go and some comment tags
to allow users to create GDExtensions in go.  One big caveat up to this point has been that this was all done
against a specific version of godot.  For this framework to be usable by a large audience, we need to expand what
versions of Godot this works for. 

Plan
====
My understanding is there's an interface that doesn't change much/often, and then there's the set of classes that
are very specific to a version of Godot.  I would like us to figure out what the minimal version of the godot header
files we need to support in order to support the maximum version of godot today.  I'd like this repository to commit
no generated code and have all the tooling needed to generate the classes and supporting functions from scratch.

Ideally, the user would first start their new go project, create a `.go` file that serves as the initial code generator
that generates all the classes, enums, etc. for a particular `extension_api.json` that they provide.  The `.go file`
would simply have a `//go:generate` and also provide the `package` name to be used for the generated files.  The godot
class files will all be generated in this one package, so no more aliases are needed.  This generate would only need to
be done once to set up their project.

Once the user has generated the initial godot bindings, they can begin building their project, and then use the usual
`go:generate` to generate bindings for their GDExtension structs.

Any existing generated classes (e.g. variants,enums, core, etc) should be moved/regenerated to godot (where we will
generate all godot bindings for testing) and gitignored so that we no longer commit the generated bindings.
