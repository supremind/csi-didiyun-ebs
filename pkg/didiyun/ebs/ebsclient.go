package ebs

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/didiyun/didiyun-go-sdk/base/v1"
	"github.com/didiyun/didiyun-go-sdk/compute/v1"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"k8s.io/klog"
)

const (
	endpoint     = "open.didiyunapi.com:8080"
	pollInterval = 3 * time.Second
)

type ebsClientInterface interface {
	Create(ctx context.Context, regionID, zoneID, name, typ string, sizeGB int64) (string, error)
	Delete(ctx context.Context, ebsUUID string) error
	Attach(ctx context.Context, ebsUUID, dc2Name string) (string, error)
	Detach(ctx context.Context, ebsUUID string) error
}

type ebsClient struct {
	cli compute.EbsClient
	job compute.CommonClient
	dc2 compute.Dc2Client
}

func newEbsClient(token string, to time.Duration) (ebsClientInterface, error) {
	cred := oauth.NewOauthAccess(&oauth2.Token{
		AccessToken: token,
		TokenType:   "bearer",
	})
	conn, e := grpc.Dial(endpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithTimeout(to),
	)
	if e != nil {
		return nil, e
	}
	return &ebsClient{
		cli: compute.NewEbsClient(conn),
		job: compute.NewCommonClient(conn),
		dc2: compute.NewDc2Client(conn),
	}, nil
}

func (t *ebsClient) Create(ctx context.Context, regionID, zoneID, name, typ string, sizeGB int64) (string, error) {
	klog.V(4).Infof("creating ebs %s, type %s, size %d GB", name, typ, sizeGB)
	req := &compute.CreateEbsRequest{
		Header:       &base.Header{RegionId: regionID, ZoneId: zoneID},
		Count:        1,
		AutoContinue: false,
		PayPeriod:    0,
		Name:         name,
		Size:         sizeGB,
		DiskType:     typ,
	}
	resp, e := t.cli.CreateEbs(ctx, req)
	if e != nil {
		return "", fmt.Errorf("create ebs error %w", e)
	}
	if resp.Error.Errno != 0 {
		return "", fmt.Errorf("create ebs error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}

	job, e := t.waitForJob(ctx, resp.Data[0], regionID, zoneID)
	if e != nil {
		return "", e
	}
	if !job.Success {
		if job.ResourceUuid != "" { // not success, but still got uuid that already created
			return job.ResourceUuid, nil
		}
		return "", fmt.Errorf("failed to create ebs: %s", job.Result)
	}
	return job.ResourceUuid, nil
}

func (t *ebsClient) attachedDevice(ctx context.Context, ebsUUID, dc2Name string) (string, error) {
	klog.V(4).Infof("getting ebs %s", ebsUUID)
	req := &compute.GetEbsByUuidRequest{
		EbsUuid: ebsUUID,
	}
	resp, e := t.cli.GetEbsByUuid(ctx, req)
	if e != nil {
		return "", fmt.Errorf("get ebs error %w", e)
	}
	if resp.Error.Errno != 0 {
		return "", fmt.Errorf("get ebs error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}

	if len(resp.Data) == 0 {
		klog.V(4).Infof("ebs %s not found", ebsUUID)
		return "", nil
	}

	dc2 := resp.Data[0].GetDc2()
	if dc2 == nil {
		klog.V(4).Infof("ebs %s is already detached", ebsUUID)
		return "", nil
	}
	dn := resp.Data[0].GetDeviceName()
	klog.V(4).Infof("ebs %s is already attached to %s, device %s", ebsUUID, dc2.GetName(), dn)

	// not attached to target dc2
	if dc2Name != "" && dc2.GetName() != dc2Name {
		return "", nil
	}
	// attached to any dc2
	return dn, nil
}

func (t *ebsClient) Delete(ctx context.Context, ebsUUID string) error {
	klog.V(4).Infof("deleting ebs %s", ebsUUID)
	req := &compute.DeleteEbsRequest{
		Ebs: []*compute.DeleteEbsRequest_Input{{EbsUuid: ebsUUID}},
	}
	resp, e := t.cli.DeleteEbs(ctx, req)
	if e != nil {
		return fmt.Errorf("delete ebs error %w", e)
	}
	if resp.Error.Errno != 0 {
		return fmt.Errorf("delete ebs error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}

	job, e := t.waitForJob(ctx, resp.Data[0], "", "")
	if e != nil {
		return e
	}
	if !job.Success {
		return fmt.Errorf("failed to delete ebs: %s", job.Result)
	}
	return nil
}

func (t *ebsClient) Attach(ctx context.Context, ebsUUID, dc2Name string) (string, error) {
	klog.V(4).Infof("attaching ebs %s to dc2 %s", ebsUUID, dc2Name)
	dc2UUID, e := t.getDc2UUIDByName(ctx, dc2Name)
	if e != nil {
		return "", e
	}

	req := &compute.AttachEbsRequest{
		Ebs: []*compute.AttachEbsRequest_Input{{EbsUuid: ebsUUID, Dc2Uuid: dc2UUID}},
	}
	resp, e := t.cli.AttachEbs(ctx, req)
	if e != nil {
		return "", fmt.Errorf("attach ebs error %w", e)
	}
	if resp.Error.Errno != 0 {
		return "", fmt.Errorf("attach ebs error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}

	job, e := t.waitForJob(ctx, resp.Data[0], "", "")
	if e != nil {
		return "", e
	}

	// whether job.Success or not, need check attached device
	device, e := t.attachedDevice(ctx, ebsUUID, dc2Name)
	if e != nil {
		return "", e
	}
	if device != "" {
		return device, nil
	}
	return "", fmt.Errorf("failed to attach ebs: %s", job.Result)
}

func (t *ebsClient) Detach(ctx context.Context, ebsUUID string) error {
	klog.V(4).Infof("detaching ebs %s", ebsUUID)
	req := &compute.DetachEbsRequest{
		Ebs: []*compute.DetachEbsRequest_Input{{EbsUuid: ebsUUID}},
	}
	resp, e := t.cli.DetachEbs(ctx, req)
	if e != nil {
		return fmt.Errorf("detach ebs error %w", e)
	}
	if resp.Error.Errno != 0 {
		return fmt.Errorf("detach ebs error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}

	job, e := t.waitForJob(ctx, resp.Data[0], "", "")
	if e != nil {
		return e
	}

	device, e := t.attachedDevice(ctx, ebsUUID, "")
	if e != nil {
		return e
	}
	if device == "" {
		return nil
	}
	return fmt.Errorf("failed to detach ebs: %s", job.Result)
}

func (t *ebsClient) getDc2UUIDByName(ctx context.Context, name string) (string, error) {
	klog.V(4).Infof("getting dc2 uuid by %s", name)
	req := &compute.ListDc2Request{
		Start:     0,
		Limit:     100,
		Simplify:  true,
		Condition: &compute.ListDc2Condition{Dc2Name: name},
	}
	resp, e := t.dc2.ListDc2(ctx, req)
	if e != nil {
		return "", fmt.Errorf("get dc2 error %w", e)
	}
	if resp.Error.Errno != 0 {
		return "", fmt.Errorf("get dc2 error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
	}
	for _, d := range resp.Data {
		if d.GetName() == name {
			return d.GetDc2Uuid(), nil
		}
	}
	return "", fmt.Errorf("dc2 %s is not found", name)
}

func (t *ebsClient) waitForJob(ctx context.Context, info *base.JobInfo, regionID, zoneID string) (*base.JobInfo, error) {
	for {
		if info.Done {
			return info, nil
		}

		klog.V(5).Infof("wait for job %+v", *info)
		time.Sleep(pollInterval) // simply use a constant interval
		resp, e := t.job.JobResult(ctx, &compute.JobResultRequest{
			Header:   &base.Header{RegionId: regionID, ZoneId: zoneID},
			JobUuids: []string{info.JobUuid},
		})
		if e != nil {
			return nil, fmt.Errorf("job result error %w", e)
		}
		if resp.Error.Errno != 0 {
			return nil, fmt.Errorf("job result error %s (%d)", resp.Error.Errmsg, resp.Error.Errno)
		}
		info = resp.Data[0]
	}
}
