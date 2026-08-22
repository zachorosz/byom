package scan

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zachorosz/byom/storage"
)

type walkResult struct {
	dir   string
	files []walkEntry
}

type walkEntry struct {
	fs.DirEntry
	kind storage.FileKind
}

type prefixedEntry struct {
	fs.DirEntry
	prefix string
}

func (e prefixedEntry) Name() string { return path.Join(e.prefix, e.DirEntry.Name()) }

func walkPostOrder(fsys fs.FS, dir string, process func(walkResult) error) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := path.Join(dir, e.Name())
		if err := walkPostOrder(fsys, child, process); err != nil {
			return err
		}
	}

	res := walkResult{dir: dir}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		kind, ok := classifyKind(e)
		if !ok {
			continue
		}
		res.files = append(res.files, walkEntry{DirEntry: e, kind: kind})
	}

	return process(res)
}

var (
	audioExtensions = map[string]bool{
		".flac": true, ".mp3": true, ".ogg": true, ".m4a": true,
		".aiff": true, ".aif": true, ".wav": true, ".opus": true,
		".ape": true, ".wv": true,
	}
	imageExtensions = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true,
	}
)

func classifyKind(e fs.DirEntry) (storage.FileKind, bool) {
	ext := strings.ToLower(filepath.Ext(e.Name()))
	switch {
	case audioExtensions[ext]:
		return storage.FileAudio, true
	case imageExtensions[ext]:
		return storage.FileImage, true
	default:
		return "", false
	}
}

var discPattern = regexp.MustCompile(`^(?i)(?:cd|disc)[\s._-]*\d+(?:[\s._-].*)?$`)

// isDisc reports whether dir's base name denotes a disc folder, such as "CD1"
// or "Disc 2 - Remixes".
func isDisc(dir string) bool {
	return discPattern.MatchString(path.Base(dir))
}

type discMerger struct {
	// pending holds walked dirs until their parent is seen.
	// fixme: leftover discs are never flushed.
	pending map[string]walkResult
	emit    func(walkResult) error
}

func newDiscMerger(emit func(walkResult) error) *discMerger {
	return &discMerger{
		pending: make(map[string]walkResult),
		emit:    emit,
	}
}

// process buffers disc dirs (Album/CD1) and folds them into their
// parent album when the post-order walk reaches it, so an album emits
// once with its discs' files prefixed by the disc folder name.
func (dm *discMerger) process(res walkResult) error {
	if isDisc(res.dir) {
		dm.pending[res.dir] = res
		return nil
	}

	for dir, disc := range dm.pending {
		if path.Dir(dir) != res.dir {
			continue
		}
		delete(dm.pending, dir)

		discName := filepath.Base(disc.dir)
		for _, f := range disc.files {
			res.files = append(res.files, walkEntry{
				DirEntry: prefixedEntry{DirEntry: f.DirEntry, prefix: discName},
				kind:     f.kind,
			})
		}
	}

	return dm.emit(res)
}
