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
