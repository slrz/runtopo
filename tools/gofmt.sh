#!/bin/sh
gofiles=$(go list -f '{{$d := .Dir }}{{range $f := .GoFiles}}{{printf "%s/%s\n" $d $f}}{{end}}' ./...)
misformatted=$(gofmt -l $gofiles)

test -z "$misformatted" && exit 0

echo "$misformatted"
exit 1
