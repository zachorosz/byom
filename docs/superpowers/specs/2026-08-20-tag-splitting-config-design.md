# Per-Tag Splitting Config

## Problem

`normTags` (in `metadata/tags.go`) currently applies the same delimiter-splitting
logic to every tag value, regardless of the tag. This is wrong for two reasons:

1. Some tags are inherently single-valued free text (`ALBUM`, `TITLE`,
   `DISCSUBTITLE`, ...). Splitting them on `;` or `\` risks corrupting values
   that happen to contain those characters.
2. `" / "` was previously included as a global split delimiter, which broke
   band names containing a literal slash (e.g. `AC/DC`). It's really an
   artist-name convention ("Artist A / Artist B"), not a generic tag
   delimiter, so it doesn't belong in the generic splitting path at all.

## Design

### `normTags` / `splitValues`: allowlist-gated splitting

`normTags` splits a tag's values only if the tag is on a `splittableTags`
allowlist. Unlisted tags default to **no split** — this is deliberately the
safer default, since an unrecognized single-value tag is more common than an
unrecognized multi-value one, and a wrongly-split value is harder to recover
from than a wrongly-unsplit one.

```go
var splittableTags = map[string]bool{
	"ARTIST":      true,
	"ALBUMARTIST": true,
	"ARRANGER":    true,
	"COMPOSER":    true,
	"CONDUCTOR":   true,
	"ENGINEER":    true,
	"LYRICIST":    true,
	"PERFORMER":   true,
	"PRODUCER":    true,
	"PERSONNEL":   true,
	"LABEL":       true,
}

var splitDelims = []string{";", `\\`}
```

All splittable tags share the same delimiter set — no per-tag delimiter
config. `" / "` is removed from this set entirely (see below).

```go
func normTags(tags map[string][]string) map[string][]string {
	norm := map[string][]string{}
	for k, v := range tags {
		key := normTagKey(k)
		cleaned := cleanValues(v)
		if splittableTags[key] {
			cleaned = splitValues(cleaned, splitDelims)
		}
		norm[key] = cleaned
	}
	return norm
}
```

`cleanValues` is a new helper: strips `\x00`, trims whitespace, and drops
empty strings. It runs unconditionally on every tag's values, splittable or
not — trimming/empty-filtering is generic hygiene, not delimiter-splitting.

`splitValues` keeps its current signature and behavior (split on each
delimiter in turn, trim, drop empties) but is only ever invoked for tags on
the allowlist.

### Artist-name `" / "` split: scoped to credit/artist mapping

`" / "` splitting is reintroduced, but only where the "Artist A / Artist B"
convention is expected: the `ARTIST` and `ALBUMARTIST` tags, at the point
they're mapped into `Credit`s — not in `normTags`.

```go
func splitArtistValues(values []string) []string {
	return splitValues(values, []string{" / "})
}
```

This reuses `splitValues` for its trim/drop-empty behavior with a different
delimiter, applied after `normTags`'s `;`/`\` splitting has already run.

**`mapCredits`** (`metadata/tags.go`): `ARTIST` is pulled out of the generic
`tagRoles` loop and handled separately with `splitArtistValues`, producing
`"Performer"` credits. The rest of `tagRoles` (`ARRANGER`, `COMPOSER`,
`CONDUCTOR`, `ENGINEER`, `LYRICIST`, `PERFORMER`, `PRODUCER`) is unaffected —
those tags are not split on `" / "`.

**`mapAlbumArtists`**: applies `splitArtistValues` to both the `ALBUMARTIST`
lookup and the `ARTIST` fallback.

Behavioral note: today, the `ARTIST`-fallback path in `mapAlbumArtists`
triggers when there is exactly one raw `ARTIST` value. After this change, a
single raw value like `"Artist A / Artist B"` is split into two names before
that check runs, so `len(names) != 1` correctly treats it as ambiguous
(multiple artists, no unambiguous album artist to infer) and returns no
credits, instead of incorrectly using the whole unsplit string as one album
artist name.

## Testing

- `TestNormTags` (new, `metadata/tags_test.go`): a splittable tag value with
  `;` and `\` splits as expected; a non-splittable tag (e.g. `ALBUM`) with an
  embedded `;` is left intact; an unrecognized tag defaults to no split.
- `TestMapCredits` (new): `ARTIST: ["Artist A / Artist B"]` produces two
  `"Performer"` credits; `ARTIST: ["AC/DC"]` produces one credit with the `/`
  preserved; a role tag such as `COMPOSER: ["Foo / Bar"]` stays unsplit (one
  credit, literal `/` intact).
- `TestMapAlbumArtists` (new): `ALBUMARTIST` present is split into multiple
  artists; `ALBUMARTIST` absent with a single `ARTIST` value falls back
  correctly; `ALBUMARTIST` absent with `ARTIST: ["A / B"]` returns no credits
  (ambiguous, no fallback).
- Existing `TestSplitValues` and `TestParseCredit` are unaffected and stay as
  direct delimiter-mechanics tests.

## Out of scope

- No per-tag custom delimiter sets (all splittable tags share one delimiter
  list).
- No changes to tags not currently read anywhere in `metadata/` (this design
  only classifies tags that are read today, plus `LABEL` per explicit
  request, even though `LABEL` isn't consumed downstream yet).
