#!/bin/sh

coverdir=coverage
covermode=count
coverout="$coverdir/cover.out"
coverhtml="$coverdir/cover.html"
coberturaxmlout="$coverdir/coverage.xml"

runtests() {
	mkdir -p "$coverdir"
	for p in $(go list ./...); do
		go test -covermode "$covermode" -coverprofile "$coverdir/${p##*/}.out" "$p"
	done

	echo "mode: $covermode" >"$coverout"
	tail -qn+2 "$coverdir"/*.out >>"$coverout"
	go tool cover -func "$coverout"
}

case "$1" in
cover)
	runtests
	;;
coverhtml)
	go tool cover -o "$coverhtml" -html "$coverout"
	;;
coberturaxml)
	gocover-cobertura <"$coverout" >"$coberturaxmlout"
	;;
*)
	exit 2
	;;
esac
