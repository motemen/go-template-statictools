# Go template static tools

## gotmplcheck

Statically typechecks Go templates.

    go install github.com/motemen/go-template-statictools/cmd/gotmplcheck@latest

### Usage

    gotmplcheck [-dot path/to/pkg.type] [-funcmap path/to/pkg.var] [-soft] [-verbose] template.tmpl

`-dot` specifies the type of the data passed to the template. It can be specified in the template itself with `{{/* @type path/to/pkg */}}`.

`-funcmap` specifies the function map passed to the template.

`-soft` ignores errors about undefined functions and templates.

`-verbose` prints verbose information.
