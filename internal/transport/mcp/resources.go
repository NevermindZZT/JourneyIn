package mcptransport

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) registerResources() {
	if s.schema != nil {
		s.mcp.AddResource(&mcp.Resource{URI: "journeyin://schema/trip/v1", Name: "JourneyIn Trip Schema v1", Description: "Canonical JSON schema for JourneyIn trips", MIMEType: "application/schema+json"}, s.readSchema)
	}
	s.mcp.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "journeyin://trips/{trip_id}", Name: "JourneyIn Trip", Description: "Read a JourneyIn trip by ID", MIMEType: "application/json"}, s.readTripResource)
	s.mcp.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "journeyin://trips/{trip_id}/history/{history_id}", Name: "JourneyIn Trip History", Description: "Read a user-saved JourneyIn trip history version", MIMEType: "application/json"}, s.readTripHistoryResource)
}

func (s *Server) readSchema(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	data, err := fs.ReadFile(s.schema, "trip.v1.json")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/schema+json", Text: string(data)}}}, nil
}

func (s *Server) readTripResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	id := strings.TrimPrefix(req.Params.URI, "journeyin://trips/")
	if id == "" || strings.Contains(id, "/") {
		return nil, fmt.Errorf("invalid trip resource URI")
	}
	record, err := s.app.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(record.Document)}}}, nil
}

func (s *Server) readTripHistoryResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	path := strings.TrimPrefix(req.Params.URI, "journeyin://trips/")
	parts := strings.Split(path, "/history/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0], "/") || strings.Contains(parts[1], "/") {
		return nil, fmt.Errorf("invalid trip history resource URI")
	}
	record, err := s.app.GetTripVersion(ctx, parts[0], parts[1])
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(record.Document)}}}, nil
}
