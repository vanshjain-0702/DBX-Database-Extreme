package query

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dbx/dbx/internal/engine"
	"github.com/dbx/dbx/internal/protocol"
)

type vectorFlags struct {
	withDocs       string
	filterContains string
	minScore       float32
	hasMinScore    bool
	ef             int
	space          string
	weights        []float32
}

type vectorFlagOpts struct {
	allowSpace   bool
	allowWeights bool
}

func stripVectorFlags(cmd *protocol.Command, opts vectorFlagOpts) (int, vectorFlags, error) {
	end := cmd.NumArgs()
	var flags vectorFlags
	for end >= 2 {
		flag := strings.ToUpper(cmd.Arg(end - 2))
		value := cmd.Arg(end - 1)
		switch flag {
		case "WITHDOCS":
			flags.withDocs = value
			end -= 2
		case "FILTER_CONTAINS":
			flags.filterContains = value
			end -= 2
		case "MIN_SCORE":
			parsed, err := strconv.ParseFloat(value, 32)
			if err != nil {
				return 0, flags, fmt.Errorf("MIN_SCORE is not a valid float")
			}
			flags.minScore = float32(parsed)
			flags.hasMinScore = true
			end -= 2
		case "EF":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return 0, flags, fmt.Errorf("EF is not a valid positive integer")
			}
			flags.ef = parsed
			end -= 2
		case "SPACE":
			if !opts.allowSpace {
				return 0, flags, fmt.Errorf("SPACE must appear as a query segment on VFUSE")
			}
			if err := engine.ValidateSpaceName(value); err != nil {
				return 0, flags, err
			}
			flags.space = value
			end -= 2
		case "WEIGHTS":
			if !opts.allowWeights {
				return 0, flags, fmt.Errorf("WEIGHTS is only valid on VFUSE")
			}
			parts := strings.Split(value, ",")
			if len(parts) == 0 {
				return 0, flags, fmt.Errorf("WEIGHTS must be a comma-separated list")
			}
			weights := make([]float32, len(parts))
			for i, part := range parts {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
				if err != nil {
					return 0, flags, fmt.Errorf("WEIGHTS is not a valid float list")
				}
				weights[i] = float32(parsed)
			}
			flags.weights = weights
			end -= 2
		default:
			return end, flags, nil
		}
	}
	return end, flags, nil
}

func parseOptionalSpace(cmd *protocol.Command, at int) (string, int, error) {
	if at+1 >= cmd.NumArgs() || !strings.EqualFold(cmd.Arg(at), "SPACE") {
		return "", at, nil
	}
	name := cmd.Arg(at + 1)
	if err := engine.ValidateSpaceName(name); err != nil {
		return "", 0, err
	}
	return name, at + 2, nil
}

func parseFloatArgs(cmd *protocol.Command, start, end int) ([]float32, error) {
	if end < start {
		return nil, fmt.Errorf("invalid vector argument range")
	}
	vec := make([]float32, end-start)
	for i := start; i < end; i++ {
		val, err := strconv.ParseFloat(cmd.Arg(i), 32)
		if err != nil {
			return nil, fmt.Errorf("value is not a valid float: '%s'", cmd.Arg(i))
		}
		vec[i-start] = float32(val)
	}
	return vec, nil
}

func parseFuseQueries(cmd *protocol.Command, end int) (string, int, []engine.SpaceQuery, error) {
	if end < 2 {
		return "", 0, nil, fmt.Errorf("wrong number of arguments for 'VFUSE' command")
	}
	index := cmd.Arg(0)
	k, err := strconv.Atoi(cmd.Arg(end - 1))
	if err != nil {
		return "", 0, nil, fmt.Errorf("k is not a valid integer")
	}
	var queries []engine.SpaceQuery
	i := 1
	for i < end-1 {
		if !strings.EqualFold(cmd.Arg(i), "SPACE") {
			return "", 0, nil, fmt.Errorf("VFUSE expected SPACE name floats...")
		}
		i++
		if i >= end-1 {
			return "", 0, nil, fmt.Errorf("VFUSE missing space name")
		}
		name := cmd.Arg(i)
		if err := engine.ValidateSpaceName(name); err != nil {
			return "", 0, nil, err
		}
		i++
		start := i
		for i < end-1 && !strings.EqualFold(cmd.Arg(i), "SPACE") {
			i++
		}
		if i == start {
			return "", 0, nil, fmt.Errorf("empty query vector for SPACE %q", name)
		}
		vec, err := parseFloatArgs(cmd, start, i)
		if err != nil {
			return "", 0, nil, err
		}
		queries = append(queries, engine.SpaceQuery{Space: name, Query: vec})
	}
	if len(queries) < 2 {
		return "", 0, nil, fmt.Errorf("VFUSE requires at least two SPACE queries")
	}
	return index, k, queries, nil
}

func (e *Executor) vectorContainsFilter(index, substr string) func(string) bool {
	if substr == "" {
		return nil
	}
	return func(id string) bool {
		docKey := fmt.Sprintf("doc:%s:%s", index, id)
		entry, unlock := e.kv.GetForRead(docKey)
		if entry == nil {
			return false
		}
		defer unlock()
		if entry.Type != protocol.TypeString {
			return false
		}
		var strVal string
		switch v := entry.Value.(type) {
		case string:
			strVal = v
		case []byte:
			strVal = string(v)
		}
		return strings.Contains(strVal, substr)
	}
}

func (e *Executor) writeVectorHits(w *protocol.Writer, results []engine.SearchResult, withDocsPrefix string) error {
	w.WriteArray(len(results))
	for _, res := range results {
		if withDocsPrefix != "" {
			w.WriteArray(3)
			w.WriteBulkStringStr(res.ID)
			w.WriteBulkStringStr(fmt.Sprintf("%f", res.Score))
			docKey := fmt.Sprintf("%s:%s", withDocsPrefix, res.ID)
			entry, unlock := e.kv.GetForRead(docKey)
			if entry != nil && entry.Type == protocol.TypeString {
				var strVal string
				switch v := entry.Value.(type) {
				case string:
					strVal = v
				case []byte:
					strVal = string(v)
				}
				w.WriteBulkStringStr(strVal)
				unlock()
			} else {
				if entry != nil {
					unlock()
				}
				w.WriteNull()
			}
			continue
		}
		w.WriteArray(2)
		w.WriteBulkStringStr(res.ID)
		w.WriteBulkStringStr(fmt.Sprintf("%f", res.Score))
	}
	return nil
}

func searchOptsFromFlags(k int, flags vectorFlags, filter func(string) bool) engine.SearchOpts {
	return engine.SearchOpts{
		K:           k,
		Filter:      filter,
		MinScore:    flags.minScore,
		HasMinScore: flags.hasMinScore,
		EfSearch:    flags.ef,
	}
}
