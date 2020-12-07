// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	//"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	fwdpb "yunion.io/x/onecloud/pkg/hostman/guestman/forwarder/api"
)

type ovnMdFwdReq struct {
	pbreq   protoreflect.ProtoMessage
	pbresp  chan<- protoreflect.ProtoMessage
	errresp chan<- error
}

func (req ovnMdFwdReq) Do(ctx context.Context, ch chan<- ovnMdFwdReq) (protoreflect.ProtoMessage, error) {
	var (
		pbrespC  = make(chan protoreflect.ProtoMessage)
		errrespC = make(chan error)
		req1     = ovnMdFwdReq{
			pbreq:   req.pbreq,
			pbresp:  pbrespC,
			errresp: errrespC,
		}
	)
	select {
	case ch <- req1:
		select {
		case pbresp := <-pbrespC:
			return pbresp, nil
		case err := <-errrespC:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (req1 *ovnMdFwdReq) RespErr(ctx context.Context, err error) {
	select {
	case req1.errresp <- err:
	case <-ctx.Done():
	}
}

func (req1 *ovnMdFwdReq) RespPb(ctx context.Context, pb protoreflect.ProtoMessage) {
	select {
	case req1.pbresp <- pb:
	case <-ctx.Done():
	}
}

type ovnMdFwdService struct {
	ovnMdMan *ovnMdMan

	fwdpb.UnimplementedForwarderServer
}

func newOvnMdFwdService(man *ovnMdMan) *ovnMdFwdService {
	svc := &ovnMdFwdService{
		ovnMdMan: man,
	}
	return svc
}

func (svc *ovnMdFwdService) doPbReq(ctx context.Context, pbreq protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error) {
	mdreq := ovnMdFwdReq{
		pbreq: pbreq,
	}
	return svc.ovnMdMan.ForwardRequest(ctx, mdreq)
}

func (svc *ovnMdFwdService) Open(ctx context.Context, req *fwdpb.OpenRequest) (*fwdpb.OpenResponse, error) {
	if req.BindAddr == "" {
		req.BindAddr = "0.0.0.0"
	}
	pbresp, err := svc.doPbReq(ctx, req)
	if err != nil {
		return nil, err
	}
	return pbresp.(*fwdpb.OpenResponse), nil
}

func (svc *ovnMdFwdService) Close(ctx context.Context, req *fwdpb.CloseRequest) (*fwdpb.CloseResponse, error) {
	pbresp, err := svc.doPbReq(ctx, req)
	if err != nil {
		return nil, err
	}
	return pbresp.(*fwdpb.CloseResponse), nil
}

func (svc *ovnMdFwdService) ListByRemote(ctx context.Context, req *fwdpb.ListByRemoteRequest) (*fwdpb.ListByRemoteResponse, error) {
	pbresp, err := svc.doPbReq(ctx, req)
	if err != nil {
		return nil, err
	}
	return pbresp.(*fwdpb.ListByRemoteResponse), nil
}
