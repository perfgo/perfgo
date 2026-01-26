package perfscript

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/pprof/profile"
)

// Parser parses perf script output
type Parser struct {
	// Internal state for building the profile
	profile   *profile.Profile
	functions map[string]*profile.Function
	locations map[string]*profile.Location
	mappings  map[string]*profile.Mapping
	nextID    uint64
}

// New creates a new parser instance
func New() *Parser {
	return &Parser{
		functions: make(map[string]*profile.Function),
		locations: make(map[string]*profile.Location),
		mappings:  make(map[string]*profile.Mapping),
		nextID:    1,
	}
}

// Parse parses perf script output from an io.Reader and returns a pprof profile
func (p *Parser) Parse(reader io.Reader) (*profile.Profile, error) {
	// Initialize profile
	p.profile = &profile.Profile{
		SampleType:    []*profile.ValueType{},
		TimeNanos:     time.Now().UnixNano(),
		DurationNanos: 0,
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:        1,
	}

	scanner := bufio.NewScanner(reader)

	var currentStack []*profile.Location
	var currentEventType string
	var currentCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Sample header line
		// Format: program PID.TID 12345.123456: event:value
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "    ") {
			// If we have a previous stack, add it as a sample
			if len(currentStack) > 0 {
				p.addSample(currentStack, currentEventType, currentCount)
			}

			// Start new stack
			currentStack = nil

			// Extract event type
			parts := strings.Fields(line)
			if len(parts) < 2 {
				return nil, fmt.Errorf("invalid event line: %s", line)
			}
			currentEventType = strings.TrimSuffix(strings.TrimSpace(parts[len(parts)-1]), ":")
			if v, err := strconv.ParseInt(strings.TrimSpace(parts[len(parts)-2]), 10, 64); err == nil {
				currentCount = v
			} else {
				return nil, fmt.Errorf("invalid count: %s", parts[1])
			}
			continue
		}

		// Stack frame line
		// Format: 	ffffffffa1234567 function_name+0x12 (/path/to/binary)
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			loc := p.parseStackFrame(line)
			if loc != nil {
				currentStack = append(currentStack, loc)
			}
		}
	}

	// Add the last sample
	if len(currentStack) > 0 {
		p.addSample(currentStack, currentEventType, currentCount)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	// Update mapping ranges based on observed addresses
	p.finalizeMapping()

	return p.profile, nil
}

// parseStackFrame parses a single stack frame line and returns a location
func (p *Parser) parseStackFrame(line string) *profile.Location {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil
	}

	// Parse address
	addrStr := parts[0]
	addr, err := strconv.ParseUint(addrStr, 16, 64)
	if err != nil {
		// If parsing fails, use 0
		addr = 0
	}

	// Parse function name
	funcName := parts[1]
	// Remove offset if present (e.g., "function+0x12" -> "function")
	if idx := strings.LastIndex(funcName, "+"); idx > 0 {
		funcName = funcName[:idx]
	}

	// Parse binary path (if present)
	binaryPath := ""
	for i := 2; i < len(parts); i++ {
		part := parts[i]
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
			binaryPath = strings.TrimSuffix(strings.TrimPrefix(part, "("), ")")
			break
		}
	}

	// Get or create mapping
	var mapping *profile.Mapping
	if binaryPath != "" {
		// Use full path so pprof can find the binary for symbolization
		mapping = p.getOrCreateMapping(binaryPath)
	}

	// Get or create function
	fn := p.getOrCreateFunction(funcName)

	// Create location key
	locKey := fmt.Sprintf("%s:%d", funcName, addr)

	// Get or create location
	loc, exists := p.locations[locKey]
	if !exists {
		loc = &profile.Location{
			ID:      uint64(len(p.profile.Location) + 1),
			Mapping: mapping,
			Address: addr,
			Line: []profile.Line{
				{Function: fn},
			},
		}
		p.locations[locKey] = loc
		p.profile.Location = append(p.profile.Location, loc)
	}

	return loc
}

// getOrCreateFunction gets or creates a function
func (p *Parser) getOrCreateFunction(name string) *profile.Function {
	if fn, exists := p.functions[name]; exists {
		return fn
	}

	fn := &profile.Function{
		ID:   uint64(len(p.profile.Function) + 1),
		Name: name,
	}
	p.functions[name] = fn
	p.profile.Function = append(p.profile.Function, fn)
	return fn
}

// getOrCreateMapping gets or creates a mapping
func (p *Parser) getOrCreateMapping(filename string) *profile.Mapping {
	if m, exists := p.mappings[filename]; exists {
		return m
	}

	m := &profile.Mapping{
		ID:   uint64(len(p.profile.Mapping)) + 1,
		File: filename,
		// Start and Limit will be set in finalizeMapping()
	}
	p.mappings[filename] = m
	p.profile.Mapping = append(p.profile.Mapping, m)
	return m
}

// finalizeMapping sets mapping Start and Limit to allow all addresses.
// We use Start=0 and Limit=max_uint64 to pass pprof validation without
// interfering with address-to-symbol resolution. Setting Start to the
// observed minimum address would break pprof's offset calculations and
// cause incorrect symbol attribution.
func (p *Parser) finalizeMapping() {
	for _, m := range p.mappings {
		m.Start = 0
		m.Limit = ^uint64(0) // max uint64
	}
}

// addSample adds a sample with the given stack to the profile
func (p *Parser) addSample(stack []*profile.Location, eventType string, count int64) {
	if len(stack) == 0 || count == 0 {
		return
	}

	// check if the sample type already exists
	sampleIdx := -1
	for idx := range p.profile.SampleType {
		typeValue := p.profile.SampleType[idx]
		if typeValue.Type == eventType {
			sampleIdx = idx
			break
		}
	}
	if sampleIdx == -1 {
		p.profile.SampleType = append(p.profile.SampleType, &profile.ValueType{Type: eventType, Unit: "count"})
		sampleIdx = len(p.profile.SampleType) - 1
		// add a sample to each existing sample
		for _, sample := range p.profile.Sample {
			sample.Value = append(sample.Value, 0)
		}
	}

	// Check if a sample with this exact stack already exists
	for _, existingSample := range p.profile.Sample {
		if stacksEqual(existingSample.Location, stack) {
			// Merge with existing sample
			existingSample.Value[sampleIdx] += count
			return
		}
	}

	// Create new sample
	sample := &profile.Sample{
		Location: stack,
		Value:    make([]int64, len(p.profile.SampleType)),
	}
	sample.Value[sampleIdx] = count

	p.profile.Sample = append(p.profile.Sample, sample)
}

// stacksEqual returns true if two stacks have the same location IDs
func stacksEqual(a, b []*profile.Location) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}
