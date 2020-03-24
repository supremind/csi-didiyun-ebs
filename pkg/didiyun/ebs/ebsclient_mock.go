package ebs

import (
	"context"
	"fmt"

	"github.com/pborman/uuid"
)

type ebsInfo struct {
	id      string
	dc2Name string
}

type mockEbsClient struct {
	ebs map[string]*ebsInfo
}

func newMockEbsClient() ebsClientInterface {
	return &mockEbsClient{
		ebs: make(map[string]*ebsInfo),
	}
}

func (t *mockEbsClient) Create(ctx context.Context, regionID, zoneID, name, typ string, sizeGB int64) (string, error) {
	_, ok := t.ebs[name]
	if ok {
		return "", fmt.Errorf("%s already exist", name)
	}
	id := uuid.NewUUID().String()
	t.ebs[name] = &ebsInfo{id: id}
	return id, nil
}

func (t *mockEbsClient) Delete(ctx context.Context, ebsUUID string) error {
	for n, e := range t.ebs {
		if ebsUUID == e.id {
			delete(t.ebs, n)
			return nil
		}
	}
	return fmt.Errorf("%s not found", ebsUUID)
}

func (t *mockEbsClient) Attach(ctx context.Context, ebsUUID, dc2Name string) (string, error) {
	for _, e := range t.ebs {
		if ebsUUID == e.id {
			if e.dc2Name != "" && e.dc2Name != dc2Name {
				return "", fmt.Errorf("%s attached to %s", ebsUUID, e.dc2Name)
			}
			e.dc2Name = dc2Name
			return "mock-device", nil
		}
	}
	return "", fmt.Errorf("%s not found", ebsUUID)
}

func (t *mockEbsClient) Detach(ctx context.Context, ebsUUID string) error {
	for _, e := range t.ebs {
		if ebsUUID == e.id {
			e.dc2Name = ""
			return nil
		}
	}
	return fmt.Errorf("%s not found", ebsUUID)
}
