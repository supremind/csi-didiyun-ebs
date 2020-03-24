package dataset

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"git.supremind.info/products/atom/proto/go/api"
	"github.com/gogo/protobuf/jsonpb"
)

var (
	marshaler = jsonpb.Marshaler{}
)

type indexFileBuilder struct {
	file     *os.File
	rootPath string
	annoKey  string
	uriRe    *regexp.Regexp // uri regexp in annotation, e.g.: `"url":"([^ ]+)"`

	size  uint64
	count uint64
}

func newIndexFileBuilder(path, rootPath, annoKey string, uriRe *regexp.Regexp) (*indexFileBuilder, error) {
	f, e := os.Create(path)
	if e != nil {
		return nil, e
	}
	return &indexFileBuilder{file: f, annoKey: annoKey, rootPath: rootPath, uriRe: uriRe}, nil
}

func (t *indexFileBuilder) Build(input <-chan *api.DataItem, volCache *sync.Map) error {
	var size uint64
	var count uint64
	w := bufio.NewWriter(t.file)
	for item := range input {
		it, ok := item.GetAnnotations()[t.annoKey]
		if !ok {
			continue
		}
		str, e := marshaler.MarshalToString(it)
		if e != nil {
			continue
		}
		for _, m := range item.GetMetas() {
			line := str
			if t.uriRe != nil { // replace uri with local fs path
				if m.GetVolumeRef().GetKind() != api.ResourceKindVolume {
					continue
				}
				volName := m.GetVolumeRef().GetName()
				volCreator := m.GetVolumeRef().GetCreator()
				vol, ok := volCache.Load(volCreator + "/" + volName)
				if !ok { // volume cache should already be added when build dataset db
					return fmt.Errorf("%s/%s not found in cache", volCreator, volName)
				}

				vv := vol.(*api.Volume)
				path := filepath.Join(t.rootPath, vv.GetSpec().GetBucket(), vv.GetSpec().GetPath(), m.GetKey())
				line = replaceAllStringFirstSubmatchFunc(t.uriRe, str, func([]string) string {
					return path
				})
			}
			line += "\n"
			if _, e := w.Write([]byte(line)); e != nil {
				return e
			}
			size += uint64(len(line))
			count++
		}
	}
	w.Flush()
	t.size = size
	t.count = count
	return nil
}

func (t *indexFileBuilder) Size() uint64 {
	return t.size
}

func (t *indexFileBuilder) Count() uint64 {
	return t.count
}

func (t *indexFileBuilder) Close() error {
	return t.file.Close()
}

func replaceAllStringFirstSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) string) string {
	result := ""
	lastIndex := 0
	for _, v := range re.FindAllStringSubmatchIndex(str, -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}
		result += str[lastIndex:v[2]] + repl(groups)
		lastIndex = v[3]
	}
	return result + str[lastIndex:]
}
