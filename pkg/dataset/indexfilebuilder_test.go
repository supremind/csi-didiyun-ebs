package dataset

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"git.supremind.info/products/atom/proto/go/api"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
)

func TestBuild(t *testing.T) {
	label := `{
"url": "http://bucket/to/file",
"class": "cat"}`
	var labelCat types.Struct
	if e := jsonpb.UnmarshalString(label, &labelCat); e != nil {
		t.Fatal(e)
	}
	label = `{
"url": "http://bucket/to/file",
"class": "dog"}`
	var labelDog types.Struct
	if e := jsonpb.UnmarshalString(label, &labelDog); e != nil {
		t.Fatal(e)
	}

	annoKey := "labelX"
	input := make(chan *api.DataItem)
	volCache := &sync.Map{}
	go func() {
		defer close(input)
		input <- &api.DataItem{
			Metas: []*api.DatumMeta{
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test1", Creator: "admin"}, Key: "file1"},
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test1", Creator: "admin"}, Key: "file2"}},
			Annotations: map[string]*types.Struct{annoKey: &labelCat},
		}
		input <- &api.DataItem{
			Metas: []*api.DatumMeta{
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test2", Creator: "admin"}, Key: "file3"},
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test2", Creator: "admin"}, Key: "file4"}},
			Annotations: map[string]*types.Struct{annoKey: &labelDog},
		}
		input <- &api.DataItem{
			Metas: []*api.DatumMeta{
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test3", Creator: "admin"}, Key: "file5"},
				{VolumeRef: &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test3", Creator: "admin"}, Key: "file6"}},
		}
	}()
	volCache.Store("admin/test1", &api.Volume{Spec: &api.VolumeSpec{ResourceVolume: &api.ResourceVolume{Bucket: "fs/to", Path: "test1"}}})
	volCache.Store("admin/test2", &api.Volume{Spec: &api.VolumeSpec{ResourceVolume: &api.ResourceVolume{Bucket: "fs/to", Path: "test2"}}})
	volCache.Store("admin/test3", &api.Volume{Spec: &api.VolumeSpec{ResourceVolume: &api.ResourceVolume{Bucket: "fs/to", Path: "test3"}}})

	tmp, e := ioutil.TempDir("", "indexfilebuilder-test1-")
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	indexFilePath := filepath.Join(tmp, "index")

	rootPath := "/data"
	expectIndex := `{"class":"cat","url":"/data/fs/to/test1/file1"}
{"class":"cat","url":"/data/fs/to/test1/file2"}
{"class":"dog","url":"/data/fs/to/test2/file3"}
{"class":"dog","url":"/data/fs/to/test2/file4"}
` // key ordered, space, newline trimmed
	builder, e := newIndexFileBuilder(indexFilePath, rootPath, annoKey, regexp.MustCompile(`"url":"([^ ]+)"`))
	if e != nil {
		t.Fatal(e)
	}
	defer builder.Close()

	if e := builder.Build(input, volCache); e != nil {
		t.Fatal(e)
	}

	if builder.Count() != 4 {
		t.Fatalf("expect %d, got %d", 4, builder.Count())
	}
	if builder.Size() != uint64(len(expectIndex)) {
		t.Fatalf("expect %d, got %d", len(expectIndex), builder.Size())
	}

	indexFile, e := ioutil.ReadFile(indexFilePath)
	if e != nil {
		t.Fatal(e)
	}

	if string(indexFile) != expectIndex {
		t.Fatalf("expect %s, got %s", expectIndex, string(indexFile))
	}
}
