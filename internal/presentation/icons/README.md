# Interface icons

`icons.go` is generated: [Lucide](https://lucide.dev) SVGs are compiled into
`gfx.Icon` geometry, so nothing parses SVG or loads a font at run time.

- `svg/` holds the sources (Lucide, ISC licensed, see below). Drop a new SVG in
  there and rerun `bash scripts/icons.sh`.
- The script compiles them in a temporary module, so the compiler's own
  dependencies never enter this repository's `go.sum`.

## License

Icons come from [lucide-icons/lucide](https://github.com/lucide-icons/lucide),
ISC License. Copyright (c) for portions of Lucide are held by Cole Bemis
2013-2022 as part of Feather (MIT). All other copyright (c) for Lucide are held
by Lucide Contributors 2022.
