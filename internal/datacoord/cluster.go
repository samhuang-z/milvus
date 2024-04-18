// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datacoord

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus/internal/proto/datapb"
	"github.com/milvus-io/milvus/pkg/log"
	"github.com/milvus-io/milvus/pkg/util/commonpbutil"
	"github.com/milvus-io/milvus/pkg/util/paramtable"
)

// Cluster provides interfaces to interact with datanode cluster
type Cluster interface {
	Startup(ctx context.Context, nodes []*NodeInfo) error
	Register(node *NodeInfo) error
	UnRegister(node *NodeInfo) error
	Watch(ctx context.Context, ch string, collectionID UniqueID) error
	Flush(ctx context.Context, nodeID int64, channel string, segments []*datapb.SegmentInfo) error
	FlushChannels(ctx context.Context, nodeID int64, flushTs Timestamp, channels []string) error
	PreImport(nodeID int64, in *datapb.PreImportRequest) error
	ImportV2(nodeID int64, in *datapb.ImportRequest) error
	QueryPreImport(nodeID int64, in *datapb.QueryPreImportRequest) (*datapb.QueryPreImportResponse, error)
	QueryImport(nodeID int64, in *datapb.QueryImportRequest) (*datapb.QueryImportResponse, error)
	DropImport(nodeID int64, in *datapb.DropImportRequest) error
	GetSessions() []*Session
	Close()
}

var _ Cluster = (*ClusterImpl)(nil)

type ClusterImpl struct {
	sessionManager SessionManager
	channelManager ChannelManager
}

// NewClusterImpl creates a new cluster
func NewClusterImpl(sessionManager SessionManager, channelManager ChannelManager) *ClusterImpl {
	c := &ClusterImpl{
		sessionManager: sessionManager,
		channelManager: channelManager,
	}

	return c
}

// Startup inits the cluster with the given data nodes.
func (c *ClusterImpl) Startup(ctx context.Context, nodes []*NodeInfo) error {
	for _, node := range nodes {
		c.sessionManager.AddSession(node)
	}
	currs := lo.Map(nodes, func(info *NodeInfo, _ int) int64 {
		return info.NodeID
	})
	return c.channelManager.Startup(ctx, currs)
}

// Register registers a new node in cluster
func (c *ClusterImpl) Register(node *NodeInfo) error {
	c.sessionManager.AddSession(node)
	return c.channelManager.AddNode(node.NodeID)
}

// UnRegister removes a node from cluster
func (c *ClusterImpl) UnRegister(node *NodeInfo) error {
	c.sessionManager.DeleteSession(node)
	return c.channelManager.DeleteNode(node.NodeID)
}

// Watch tries to add a channel in datanode cluster
func (c *ClusterImpl) Watch(ctx context.Context, ch string, collectionID UniqueID) error {
	return c.channelManager.Watch(ctx, &channelMeta{Name: ch, CollectionID: collectionID})
}

// Flush sends async FlushSegments requests to dataNodes
// which also according to channels where segments are assigned to.
func (c *ClusterImpl) Flush(ctx context.Context, nodeID int64, channel string, segments []*datapb.SegmentInfo) error {
	if !c.channelManager.Match(nodeID, channel) {
		log.Warn("node is not matched with channel",
			zap.String("channel", channel),
			zap.Int64("nodeID", nodeID),
		)
		return fmt.Errorf("channel %s is not watched on node %d", channel, nodeID)
	}

	_, collID := c.channelManager.GetCollectionIDByChannel(channel)

	getSegmentID := func(segment *datapb.SegmentInfo, _ int) int64 {
		return segment.GetID()
	}

	req := &datapb.FlushSegmentsRequest{
		Base: commonpbutil.NewMsgBase(
			commonpbutil.WithMsgType(commonpb.MsgType_Flush),
			commonpbutil.WithSourceID(paramtable.GetNodeID()),
			commonpbutil.WithTargetID(nodeID),
		),
		CollectionID: collID,
		SegmentIDs:   lo.Map(segments, getSegmentID),
		ChannelName:  channel,
	}

	c.sessionManager.Flush(ctx, nodeID, req)
	return nil
}

func (c *ClusterImpl) FlushChannels(ctx context.Context, nodeID int64, flushTs Timestamp, channels []string) error {
	if len(channels) == 0 {
		return nil
	}

	for _, channel := range channels {
		if !c.channelManager.Match(nodeID, channel) {
			return fmt.Errorf("channel %s is not watched on node %d", channel, nodeID)
		}
	}

	req := &datapb.FlushChannelsRequest{
		Base: commonpbutil.NewMsgBase(
			commonpbutil.WithSourceID(paramtable.GetNodeID()),
			commonpbutil.WithTargetID(nodeID),
		),
		FlushTs:  flushTs,
		Channels: channels,
	}

	return c.sessionManager.FlushChannels(ctx, nodeID, req)
}

func (c *ClusterImpl) PreImport(nodeID int64, in *datapb.PreImportRequest) error {
	return c.sessionManager.PreImport(nodeID, in)
}

func (c *ClusterImpl) ImportV2(nodeID int64, in *datapb.ImportRequest) error {
	return c.sessionManager.ImportV2(nodeID, in)
}

func (c *ClusterImpl) QueryPreImport(nodeID int64, in *datapb.QueryPreImportRequest) (*datapb.QueryPreImportResponse, error) {
	return c.sessionManager.QueryPreImport(nodeID, in)
}

func (c *ClusterImpl) QueryImport(nodeID int64, in *datapb.QueryImportRequest) (*datapb.QueryImportResponse, error) {
	return c.sessionManager.QueryImport(nodeID, in)
}

func (c *ClusterImpl) DropImport(nodeID int64, in *datapb.DropImportRequest) error {
	return c.sessionManager.DropImport(nodeID, in)
}

// GetSessions returns all sessions
func (c *ClusterImpl) GetSessions() []*Session {
	return c.sessionManager.GetSessions()
}

// Close releases resources opened in Cluster
func (c *ClusterImpl) Close() {
	c.sessionManager.Close()
	c.channelManager.Close()
}
