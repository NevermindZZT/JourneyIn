package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

const (
	maxMergeRootMarkdown = 20000
	maxMergeDayMarkdown  = 10000
	maxMergeStopMarkdown = 10000
	maxMergeDays         = 60
	maxMergeStopsPerDay  = 100
	maxMergeRootLinks    = 100
	maxMergeStopLinks    = 50
	maxMergePatchBytes   = 1 << 20
)

// MergePatch is the intentionally small set of fields that a merge preview
// may change. All other Trip fields are preserved from the stored document.
type MergePatch struct {
	DescriptionMarkdown *string         `json:"description_markdown,omitempty"`
	Links               *MergeLinkPatch `json:"links,omitempty"`
	Days                []MergeDayPatch `json:"days,omitempty"`
}

// IsEmpty reports whether the patch contains no requested field changes.
func (p MergePatch) IsEmpty() bool {
	return p.DescriptionMarkdown == nil && p.Links == nil && len(p.Days) == 0
}

// MergeDayPatch addresses a Day by stable ID rather than array position.
type MergeDayPatch struct {
	DayID         string           `json:"day_id"`
	NotesMarkdown *string          `json:"notes_markdown,omitempty"`
	Stops         []MergeStopPatch `json:"stops,omitempty"`
}

// MergeStopPatch addresses a Stop within its Day by stable ID.
type MergeStopPatch struct {
	StopID              string          `json:"stop_id"`
	DescriptionMarkdown *string         `json:"description_markdown,omitempty"`
	Links               *MergeLinkPatch `json:"links,omitempty"`
}

// MergeLinkPatch uses explicit add/remove operations so omitting links never
// replaces the existing source list by accident.
type MergeLinkPatch struct {
	Add       []PatchLink `json:"add,omitempty"`
	RemoveIDs []string    `json:"remove_ids,omitempty"`
}

// PatchLink is the MCP-facing, schema-visible form of a Link addition.
type PatchLink struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Kind  string `json:"kind,omitempty"`
}

// PreviewChange is a bounded, human-readable change in a merge preview.
type PreviewChange struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (p *MergePatch) UnmarshalJSON(data []byte) error {
	type plain MergePatch
	var decoded plain
	if err := decodeStrictMergeObject(data, map[string]struct{}{
		"description_markdown": {},
		"links":                {},
		"days":                 {},
	}, &decoded, "patch"); err != nil {
		return err
	}
	*p = MergePatch(decoded)
	return nil
}

func (p *MergeDayPatch) UnmarshalJSON(data []byte) error {
	type plain MergeDayPatch
	var decoded plain
	if err := decodeStrictMergeObject(data, map[string]struct{}{
		"day_id":         {},
		"notes_markdown": {},
		"stops":          {},
	}, &decoded, "day patch"); err != nil {
		return err
	}
	*p = MergeDayPatch(decoded)
	return nil
}

func (p *MergeStopPatch) UnmarshalJSON(data []byte) error {
	type plain MergeStopPatch
	var decoded plain
	if err := decodeStrictMergeObject(data, map[string]struct{}{
		"stop_id":              {},
		"description_markdown": {},
		"links":                {},
	}, &decoded, "stop patch"); err != nil {
		return err
	}
	*p = MergeStopPatch(decoded)
	return nil
}

func (p *MergeLinkPatch) UnmarshalJSON(data []byte) error {
	type plain MergeLinkPatch
	var decoded plain
	if err := decodeStrictMergeObject(data, map[string]struct{}{
		"add":        {},
		"remove_ids": {},
	}, &decoded, "links patch"); err != nil {
		return err
	}
	*p = MergeLinkPatch(decoded)
	return nil
}

func (p *PatchLink) UnmarshalJSON(data []byte) error {
	type plain PatchLink
	var decoded plain
	if err := decodeStrictMergeObject(data, map[string]struct{}{
		"id":    {},
		"title": {},
		"url":   {},
		"kind":  {},
	}, &decoded, "link patch"); err != nil {
		return err
	}
	*p = PatchLink(decoded)
	return nil
}

func decodeStrictMergeObject(data []byte, allowed map[string]struct{}, target any, name string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%s must be a JSON object: %w", name, err)
	}
	if raw == nil {
		return fmt.Errorf("%s must be a JSON object", name)
	}
	for key, value := range raw {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%s.%s is not supported", name, key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s.%s must not be null", name, key)
		}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func (s *TripService) PreviewMerge(ctx context.Context, patch MergePatch, targetID string, expectedRevision int, createdBy string) (PreviewResult, error) {
	if strings.TrimSpace(targetID) == "" {
		return PreviewResult{}, errors.New("target_trip_id is required for merge")
	}
	if expectedRevision < 1 {
		return PreviewResult{}, errors.New("expected_revision is required for merge")
	}

	record, err := s.Get(ctx, targetID)
	if err != nil {
		return PreviewResult{}, err
	}
	if record.Revision != expectedRevision {
		return PreviewResult{}, store.ErrRevisionConflict
	}

	base, _, baseIssues, err := domain.NormalizeTripForID(record.Document, targetID)
	if err != nil {
		return PreviewResult{}, err
	}
	if hasErrors(baseIssues) {
		return PreviewResult{}, ValidationError{Issues: baseIssues}
	}

	merged, changes, err := applyRestrictedMergePatch(base, patch)
	if err != nil {
		return PreviewResult{}, err
	}
	if len(changes) == 0 {
		return PreviewResult{}, errors.New("merge patch does not change any field")
	}

	normalized, trip, issues, err := domain.NormalizeTripForID(merged, targetID)
	if err != nil {
		return PreviewResult{}, err
	}
	if hasErrors(issues) {
		return PreviewResult{}, ValidationError{Issues: issues}
	}

	preserved, err := mergePreservedSections(base, normalized)
	if err != nil {
		return PreviewResult{}, err
	}
	if !preserved["route_geometry"] || !preserved["legs"] || !preserved["map"] || !preserved["locations"] || !preserved["weather"] {
		return PreviewResult{}, errors.New("merge patch changed protected Trip data")
	}

	previewID, err := opaqueID("preview")
	if err != nil {
		return PreviewResult{}, err
	}
	token, err := opaqueID("confirm")
	if err != nil {
		return PreviewResult{}, err
	}
	now := time.Now().UTC()
	p := preview{
		ID:               previewID,
		ConfirmationHash: domain.ContentHash([]byte(token)),
		PayloadHash:      domain.ContentHash(normalized),
		Document:         normalized,
		Operation:        "merge",
		TargetTripID:     targetID,
		ExpectedRevision: expectedRevision,
		ExpiresAt:        now.Add(15 * time.Minute),
		CreatedBy:        createdBy,
	}
	s.mu.Lock()
	s.previews[previewID] = p
	s.mu.Unlock()

	return PreviewResult{
		PreviewID:            previewID,
		ExpiresAt:            p.ExpiresAt.Format(time.RFC3339),
		RequiresConfirmation: true,
		ConfirmationToken:    token,
		Summary:              trip.Summary(),
		Warnings:             warnings(issues),
		Operation:            "merge",
		TargetTripID:         targetID,
		BaseRevision:         expectedRevision,
		ChangedPaths:         changePaths(changes),
		Diff:                 changes,
		Preserved:            preserved,
	}, nil
}

func validateMergePatch(patch MergePatch) error {
	if patch.DescriptionMarkdown == nil && patch.Links == nil && len(patch.Days) == 0 {
		return errors.New("patch must contain at least one supported field")
	}
	encoded, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("encode merge patch: %w", err)
	}
	if len(encoded) > maxMergePatchBytes {
		return fmt.Errorf("merge patch exceeds the %d byte limit", maxMergePatchBytes)
	}
	if patch.DescriptionMarkdown != nil {
		if err := validateMergeMarkdown("patch.description_markdown", *patch.DescriptionMarkdown, maxMergeRootMarkdown); err != nil {
			return err
		}
	}
	if patch.Links != nil {
		if err := validateMergeLinks("patch.links", patch.Links); err != nil {
			return err
		}
	}
	if len(patch.Days) > maxMergeDays {
		return fmt.Errorf("patch.days cannot contain more than %d items", maxMergeDays)
	}
	seenDays := make(map[string]struct{}, len(patch.Days))
	for dayIndex, day := range patch.Days {
		if err := validateMergeID(fmt.Sprintf("patch.days[%d].day_id", dayIndex), day.DayID); err != nil {
			return err
		}
		if _, exists := seenDays[day.DayID]; exists {
			return fmt.Errorf("patch.days contains duplicate day_id %q", day.DayID)
		}
		seenDays[day.DayID] = struct{}{}
		if day.NotesMarkdown == nil && len(day.Stops) == 0 {
			return fmt.Errorf("patch.days[%d] must contain notes_markdown or stops", dayIndex)
		}
		if day.NotesMarkdown != nil {
			if err := validateMergeMarkdown(fmt.Sprintf("patch.days[%d].notes_markdown", dayIndex), *day.NotesMarkdown, maxMergeDayMarkdown); err != nil {
				return err
			}
		}
		if len(day.Stops) > maxMergeStopsPerDay {
			return fmt.Errorf("patch.days[%d].stops cannot contain more than %d items", dayIndex, maxMergeStopsPerDay)
		}
		seenStops := make(map[string]struct{}, len(day.Stops))
		for stopIndex, stop := range day.Stops {
			if err := validateMergeID(fmt.Sprintf("patch.days[%d].stops[%d].stop_id", dayIndex, stopIndex), stop.StopID); err != nil {
				return err
			}
			if _, exists := seenStops[stop.StopID]; exists {
				return fmt.Errorf("patch.days[%d].stops contains duplicate stop_id %q", dayIndex, stop.StopID)
			}
			seenStops[stop.StopID] = struct{}{}
			if stop.DescriptionMarkdown == nil && stop.Links == nil {
				return fmt.Errorf("patch.days[%d].stops[%d] must contain description_markdown or links", dayIndex, stopIndex)
			}
			if stop.DescriptionMarkdown != nil {
				if err := validateMergeMarkdown(fmt.Sprintf("patch.days[%d].stops[%d].description_markdown", dayIndex, stopIndex), *stop.DescriptionMarkdown, maxMergeStopMarkdown); err != nil {
					return err
				}
			}
			if stop.Links != nil {
				if err := validateMergeLinks(fmt.Sprintf("patch.days[%d].stops[%d].links", dayIndex, stopIndex), stop.Links); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateMergeLinks(path string, patch *MergeLinkPatch) error {
	if len(patch.Add) == 0 && len(patch.RemoveIDs) == 0 {
		return fmt.Errorf("%s must contain add or remove_ids", path)
	}
	if len(patch.Add) > maxMergeRootLinks || len(patch.RemoveIDs) > maxMergeRootLinks {
		return fmt.Errorf("%s contains too many link operations", path)
	}
	seen := make(map[string]struct{}, len(patch.RemoveIDs))
	for i, link := range patch.Add {
		if err := validatePatchLink(fmt.Sprintf("%s.add[%d]", path, i), link); err != nil {
			return err
		}
	}
	for i, id := range patch.RemoveIDs {
		if err := validateMergeID(fmt.Sprintf("%s.remove_ids[%d]", path, i), id); err != nil {
			return err
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%s.remove_ids contains duplicate ID %q", path, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validatePatchLink(path string, link PatchLink) error {
	if link.ID != "" {
		if err := validateMergeID(path+".id", link.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(link.Title) == "" {
		return fmt.Errorf("%s.title is required", path)
	}
	if utf8.RuneCountInString(link.Title) > 200 {
		return fmt.Errorf("%s.title is too long", path)
	}
	if utf8.RuneCountInString(link.URL) > 2000 {
		return fmt.Errorf("%s.url is too long", path)
	}
	parsed, err := url.Parse(link.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%s.url must be an absolute http/https URL", path)
	}
	if utf8.RuneCountInString(link.Kind) > 64 {
		return fmt.Errorf("%s.kind is too long", path)
	}
	return nil
}

func validateMergeID(path, id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return fmt.Errorf("%s must be a non-empty stable ID", path)
	}
	if utf8.RuneCountInString(id) > 128 {
		return fmt.Errorf("%s is too long", path)
	}
	return nil
}

func validateMergeMarkdown(path, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s exceeds the %d character limit", path, max)
	}
	return nil
}

func applyRestrictedMergePatch(document []byte, patch MergePatch) ([]byte, []PreviewChange, error) {
	if err := validateMergePatch(patch); err != nil {
		return nil, nil, err
	}
	root, err := decodeRawObject(document, "trip document")
	if err != nil {
		return nil, nil, err
	}
	var changes []PreviewChange

	if patch.DescriptionMarkdown != nil {
		change, changed, err := setMergeMarkdown(root, "description_markdown", "description_markdown", *patch.DescriptionMarkdown, maxMergeRootMarkdown)
		if err != nil {
			return nil, nil, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	if patch.Links != nil {
		change, changed, err := applyMergeLinks(root, "links", patch.Links, "links", maxMergeRootLinks)
		if err != nil {
			return nil, nil, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	if len(patch.Days) > 0 {
		rawDays, ok := root["days"]
		if !ok {
			return nil, nil, errors.New("trip document has no days array")
		}
		days, err := decodeRawArray(rawDays, "trip.days")
		if err != nil {
			return nil, nil, err
		}
		dayIndices := make(map[string]int, len(days))
		for index, rawDay := range days {
			dayID, err := rawObjectID(rawDay, fmt.Sprintf("trip.days[%d]", index))
			if err != nil {
				return nil, nil, err
			}
			if _, exists := dayIndices[dayID]; exists {
				return nil, nil, fmt.Errorf("trip.days contains duplicate id %q", dayID)
			}
			dayIndices[dayID] = index
		}

		changedDays := false
		for _, dayPatch := range patch.Days {
			index, ok := dayIndices[dayPatch.DayID]
			if !ok {
				return nil, nil, fmt.Errorf("day %q was not found", dayPatch.DayID)
			}
			dayObject, err := decodeRawObject(days[index], "day "+dayPatch.DayID)
			if err != nil {
				return nil, nil, err
			}
			changedDay := false
			if dayPatch.NotesMarkdown != nil {
				path := fmt.Sprintf("days[%s].notes_markdown", dayPatch.DayID)
				change, changed, err := setMergeMarkdown(dayObject, "notes_markdown", path, *dayPatch.NotesMarkdown, maxMergeDayMarkdown)
				if err != nil {
					return nil, nil, err
				}
				if changed {
					changes = append(changes, change)
					changedDay = true
				}
			}
			if len(dayPatch.Stops) > 0 {
				rawStops, ok := dayObject["stops"]
				if !ok {
					return nil, nil, fmt.Errorf("day %q has no stops array", dayPatch.DayID)
				}
				stops, err := decodeRawArray(rawStops, "day "+dayPatch.DayID+".stops")
				if err != nil {
					return nil, nil, err
				}
				stopIndices := make(map[string]int, len(stops))
				for stopIndex, rawStop := range stops {
					stopID, err := rawObjectID(rawStop, fmt.Sprintf("days[%s].stops[%d]", dayPatch.DayID, stopIndex))
					if err != nil {
						return nil, nil, err
					}
					if _, exists := stopIndices[stopID]; exists {
						return nil, nil, fmt.Errorf("day %q contains duplicate stop id %q", dayPatch.DayID, stopID)
					}
					stopIndices[stopID] = stopIndex
				}

				changedStops := false
				for _, stopPatch := range dayPatch.Stops {
					stopIndex, ok := stopIndices[stopPatch.StopID]
					if !ok {
						return nil, nil, fmt.Errorf("stop %q was not found in day %q", stopPatch.StopID, dayPatch.DayID)
					}
					stopObject, err := decodeRawObject(stops[stopIndex], "stop "+stopPatch.StopID)
					if err != nil {
						return nil, nil, err
					}
					changedStop := false
					if stopPatch.DescriptionMarkdown != nil {
						path := fmt.Sprintf("days[%s].stops[%s].description_markdown", dayPatch.DayID, stopPatch.StopID)
						change, changed, err := setMergeMarkdown(stopObject, "description_markdown", path, *stopPatch.DescriptionMarkdown, maxMergeStopMarkdown)
						if err != nil {
							return nil, nil, err
						}
						if changed {
							changes = append(changes, change)
							changedStop = true
						}
					}
					if stopPatch.Links != nil {
						path := fmt.Sprintf("days[%s].stops[%s].links", dayPatch.DayID, stopPatch.StopID)
						change, changed, err := applyMergeLinks(stopObject, "links", stopPatch.Links, path, maxMergeStopLinks)
						if err != nil {
							return nil, nil, err
						}
						if changed {
							changes = append(changes, change)
							changedStop = true
						}
					}
					if changedStop {
						stops[stopIndex], err = json.Marshal(stopObject)
						if err != nil {
							return nil, nil, err
						}
						changedStops = true
					}
				}
				if changedStops {
					dayObject["stops"], err = json.Marshal(stops)
					if err != nil {
						return nil, nil, err
					}
					changedDay = true
				}
			}
			if changedDay {
				days[index], err = json.Marshal(dayObject)
				if err != nil {
					return nil, nil, err
				}
				changedDays = true
			}
		}
		if changedDays {
			root["days"], err = json.Marshal(days)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if len(changes) == 0 {
		return nil, nil, errors.New("merge patch does not change any field")
	}
	merged, err := json.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	return merged, changes, nil
}

func setMergeMarkdown(object map[string]json.RawMessage, key, path, value string, max int) (PreviewChange, bool, error) {
	if err := validateMergeMarkdown(path, value, max); err != nil {
		return PreviewChange{}, false, err
	}
	before, err := rawStringField(object, key, path)
	if err != nil {
		return PreviewChange{}, false, err
	}
	if before == value {
		return PreviewChange{}, false, nil
	}
	if value == "" {
		delete(object, key)
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return PreviewChange{}, false, err
		}
		object[key] = encoded
	}
	return PreviewChange{Path: path, Before: before, After: value}, true, nil
}

func applyMergeLinks(object map[string]json.RawMessage, key string, patch *MergeLinkPatch, path string, max int) (PreviewChange, bool, error) {
	links := make([]json.RawMessage, 0)
	var err error
	if raw, ok := object[key]; ok {
		links, err = decodeRawArray(raw, path)
		if err != nil {
			return PreviewChange{}, false, err
		}
	}
	beforeJSON, err := json.Marshal(links)
	if err != nil {
		return PreviewChange{}, false, err
	}

	linkIDs := make([]string, len(links))
	existingIDs := make(map[string]struct{}, len(links))
	for index, rawLink := range links {
		linkObject, err := decodeRawObject(rawLink, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return PreviewChange{}, false, err
		}
		id, err := rawStringField(linkObject, "id", fmt.Sprintf("%s[%d].id", path, index))
		if err != nil {
			return PreviewChange{}, false, err
		}
		linkIDs[index] = id
		if id == "" {
			continue
		}
		if _, exists := existingIDs[id]; exists {
			return PreviewChange{}, false, fmt.Errorf("%s contains duplicate link id %q", path, id)
		}
		existingIDs[id] = struct{}{}
	}
	removeSet := make(map[string]struct{}, len(patch.RemoveIDs))
	for _, id := range patch.RemoveIDs {
		if _, exists := existingIDs[id]; !exists {
			return PreviewChange{}, false, fmt.Errorf("link %q was not found at %s", id, path)
		}
		removeSet[id] = struct{}{}
	}
	updated := make([]json.RawMessage, 0, len(links)+len(patch.Add))
	seenIDs := make(map[string]struct{}, len(links)+len(patch.Add))
	for index, rawLink := range links {
		id := linkIDs[index]
		if _, removed := removeSet[id]; removed {
			continue
		}
		updated = append(updated, rawLink)
		if id != "" {
			seenIDs[id] = struct{}{}
		}
	}
	for _, addition := range patch.Add {
		link := domain.Link{ID: addition.ID, Title: addition.Title, URL: addition.URL, Kind: addition.Kind}
		if link.ID == "" {
			link.ID, err = domain.NewID("link")
			if err != nil {
				return PreviewChange{}, false, err
			}
		}
		if _, exists := seenIDs[link.ID]; exists {
			return PreviewChange{}, false, fmt.Errorf("link id %q already exists at %s", link.ID, path)
		}
		seenIDs[link.ID] = struct{}{}
		encoded, err := json.Marshal(link)
		if err != nil {
			return PreviewChange{}, false, err
		}
		updated = append(updated, encoded)
	}
	if len(updated) > max {
		return PreviewChange{}, false, fmt.Errorf("%s would contain more than %d links", path, max)
	}
	afterJSON, err := json.Marshal(updated)
	if err != nil {
		return PreviewChange{}, false, err
	}
	if bytes.Equal(beforeJSON, afterJSON) {
		return PreviewChange{}, false, nil
	}
	if len(updated) == 0 {
		delete(object, key)
	} else {
		object[key] = afterJSON
	}
	return PreviewChange{Path: path, Before: string(beforeJSON), After: string(afterJSON)}, true, nil
}

func decodeRawObject(data []byte, path string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("%s must be an object: %w", path, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	return object, nil
}

func decodeRawArray(data []byte, path string) ([]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("%s must be an array", path)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("%s must be an array: %w", path, err)
	}
	return values, nil
}

func rawObjectID(data []byte, path string) (string, error) {
	object, err := decodeRawObject(data, path)
	if err != nil {
		return "", err
	}
	id, err := rawStringField(object, "id", path+".id")
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("%s.id is required", path)
	}
	return id, nil
}

func rawStringField(object map[string]json.RawMessage, key, path string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("%s must be a string", path)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", path, err)
	}
	return value, nil
}

func changePaths(changes []PreviewChange) []string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	return paths
}

func mergePreservedSections(before, after []byte) (map[string]bool, error) {
	beforeProjection, err := protectedMergeProjection(before)
	if err != nil {
		return nil, err
	}
	afterProjection, err := protectedMergeProjection(after)
	if err != nil {
		return nil, err
	}
	unchanged := bytes.Equal(beforeProjection, afterProjection)
	return map[string]bool{
		"route_geometry": unchanged,
		"legs":           unchanged,
		"map":            unchanged,
		"locations":      unchanged,
		"weather":        unchanged,
	}, nil
}

func protectedMergeProjection(data []byte) ([]byte, error) {
	root, err := decodeRawObject(data, "trip document")
	if err != nil {
		return nil, err
	}
	delete(root, "description_markdown")
	delete(root, "links")
	if rawDays, ok := root["days"]; ok {
		days, err := decodeRawArray(rawDays, "trip.days")
		if err != nil {
			return nil, err
		}
		for dayIndex, rawDay := range days {
			dayObject, err := decodeRawObject(rawDay, fmt.Sprintf("trip.days[%d]", dayIndex))
			if err != nil {
				return nil, err
			}
			delete(dayObject, "notes_markdown")
			if rawStops, ok := dayObject["stops"]; ok {
				stops, err := decodeRawArray(rawStops, fmt.Sprintf("trip.days[%d].stops", dayIndex))
				if err != nil {
					return nil, err
				}
				for stopIndex, rawStop := range stops {
					stopObject, err := decodeRawObject(rawStop, fmt.Sprintf("trip.days[%d].stops[%d]", dayIndex, stopIndex))
					if err != nil {
						return nil, err
					}
					delete(stopObject, "description_markdown")
					delete(stopObject, "links")
					stops[stopIndex], err = json.Marshal(stopObject)
					if err != nil {
						return nil, err
					}
				}
				dayObject["stops"], err = json.Marshal(stops)
				if err != nil {
					return nil, err
				}
			}
			days[dayIndex], err = json.Marshal(dayObject)
			if err != nil {
				return nil, err
			}
		}
		root["days"], err = json.Marshal(days)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(root)
}
