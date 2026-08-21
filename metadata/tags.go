package metadata

import (
	"strconv"
	"strings"

	"github.com/zachorosz/byom/library"
)

var (
	splittableTags = map[string]bool{
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
	splitDelims = []string{";", `\\`}
)

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

func normTagKey(key string) string { return strings.ToUpper(key) }

// cleanValues strips null bytes, trims whitespace, and drops empty strings.
func cleanValues(values []string) []string {
	var cleaned []string
	for _, s := range values {
		if trimmed := strings.TrimSpace(strings.ReplaceAll(s, "\x00", "")); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func splitValues(values []string, joins []string) []string {
	result := values
	for _, delim := range joins {
		var nextRound []string
		for _, chunk := range result {
			for part := range strings.SplitSeq(chunk, delim) {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					nextRound = append(nextRound, trimmed)
				}
			}
		}
		result = nextRound
	}
	return result
}

// splitArtistValues splits values on the "Artist A / Artist B" convention.
func splitArtistValues(values []string) []string {
	return splitValues(values, []string{" / "})
}

// firstMatching looks through names in order and returns the values
// for the first key found in tags.
func firstMatching(tags map[string][]string, keys []string) []string {
	for _, k := range keys {
		if vals := tags[normTagKey(k)]; len(vals) > 0 {
			return vals
		}
	}
	return nil
}

// firstValue finds the first matching tag slice and returns its
// first non-empty string element.
func firstValue(tags map[string][]string, keys []string) string {
	for _, val := range firstMatching(tags, keys) {
		if val != "" {
			return val
		}
	}
	return ""
}

func truthy(s string) bool { return s == "1" }

// parseLeadingInt reads the leading run of digits in s.
func parseLeadingInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

func discNumber(tags map[string][]string) int {
	n, _ := parseLeadingInt(firstValue(tags, []string{"DISCNUMBER"}))
	if n <= 0 {
		return 1
	}
	return n
}

func trackNumber(tags map[string][]string) int {
	n, _ := parseLeadingInt(firstValue(tags, []string{"TRACKNUMBER"}))
	return n
}

func mapTrack(tags map[string][]string) TrackMetadata {
	return TrackMetadata{
		DiscNumber:          discNumber(tags),
		DiscSubtitle:        firstValue(tags, []string{"DISCSUBTITLE"}),
		TrackNumber:         trackNumber(tags),
		Title:               firstValue(tags, []string{"TITLE"}),
		ReleaseDate:         firstValue(tags, []string{"DATE", "YEAR"}),
		OriginalReleaseDate: firstValue(tags, []string{"ORIGINALDATE", "ORIGINALYEAR"}),
		Live:                truthy(firstValue(tags, []string{"LIVE"})),
	}
}

func mapCredits(tags map[string][]string) []Credit {
	var credits []Credit

	for _, name := range splitArtistValues(firstMatching(tags, []string{"ARTIST"})) {
		credits = append(credits, Credit{CreditedName: name, Role: "Performer"})
	}

	tagRoles := []struct {
		tag  string
		role string
	}{
		{tag: "ARRANGER", role: "Arranger"},
		{tag: "COMPOSER", role: "Composer"},
		{tag: "CONDUCTOR", role: "Conductor"},
		{tag: "ENGINEER", role: "Engineer"},
		{tag: "LYRICIST", role: "Lyricist"},
		{tag: "PERFORMER", role: "Performer"},
		{tag: "PRODUCER", role: "Producer"},
	}
	for _, tr := range tagRoles {
		for _, name := range firstMatching(tags, []string{tr.tag}) {
			credits = append(credits, Credit{CreditedName: name, Role: tr.role})
		}
	}

	for _, v := range firstMatching(tags, []string{"PERSONNEL"}) {
		if c, ok := parseCredit(v); ok {
			credits = append(credits, c)
		}
	}

	return credits
}

// parseCredit parses a "Roon"-style credit string, splitting it into a name
// and role at the first occurrence of " - ".
func parseCredit(s string) (Credit, bool) {
	name, role, found := strings.Cut(s, " - ")
	if !found {
		return Credit{}, false
	}
	name, role = strings.TrimSpace(name), strings.TrimSpace(role)
	if name == "" || role == "" {
		return Credit{}, false
	}
	return Credit{CreditedName: name, Role: role}, true
}

func mapAlbum(tags map[string][]string) AlbumMetadata {
	return AlbumMetadata{
		Type:                mapAlbumType(tags),
		Title:               firstValue(tags, []string{"ALBUM"}),
		ReleaseDate:         firstValue(tags, []string{"DATE", "YEAR"}),
		OriginalReleaseDate: firstValue(tags, []string{"ORIGINALDATE", "ORIGINALYEAR"}),
		Bootleg:             strings.EqualFold(firstValue(tags, []string{"RELEASETYPE"}), "bootleg"),
		Compilation: truthy(firstValue(tags, []string{"COMPILATION"})) ||
			strings.EqualFold(firstValue(tags, []string{"RELEASETYPE"}), "compilation"),
		Live: strings.EqualFold(firstValue(tags, []string{"RELEASETYPE"}), "live"),
	}
}

func mapAlbumType(tags map[string][]string) library.AlbumType {
	s := strings.ToLower(firstValue(tags, []string{"RELEASETYPE"}))
	switch s {
	case "album", "live", "compilation":
		return library.AlbumMain
	case "single":
		return library.AlbumSingle
	case "ep", "extendedplay":
		return library.AlbumEP
	case "other":
		return library.AlbumOther
	default:
		return library.AlbumTypeUnknown
	}
}

func mapAlbumArtists(tags map[string][]string) []Credit {
	var artists []Credit
	for _, name := range splitArtistValues(firstMatching(tags, []string{"ALBUMARTIST"})) {
		artists = append(artists, Credit{
			CreditedName: name,
		})
	}
	if len(artists) > 0 {
		return artists
	}
	// fallback to ARTIST tag if there is a single value
	names := splitArtistValues(firstMatching(tags, []string{"ARTIST"}))
	if len(names) != 1 {
		return []Credit{}
	}
	return []Credit{{CreditedName: names[0]}}
}
