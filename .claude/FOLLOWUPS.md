The main thing left to polish is generating documentation for GDExtension classes.
I'd like to model the existing documentation XML schema to provide the user with the
ability to use doc tags to affect the generated XML documentation or registration names.

We already can influence the name of the class and methods with @name, it would be nice
to have @description as well, and other tags.  I would also like to default to using the
go docs (minus doc tags) as the default description.

I would like method args to have the same name as in the go GDExtension code.

Finally, when using enums, I would like the return type or argument types to show
the enum type rather than int in the GDSCript documentation/autocompletion.

