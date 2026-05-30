package shipit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DocumentIndex struct {
	RunID       string               `json:"run_id"`
	GeneratedAt time.Time            `json:"generated_at"`
	Documents   []DocumentIndexEntry `json:"documents"`
}

type DocumentIndexEntry struct {
	Path        string    `json:"path"`
	Kind        string    `json:"kind"`
	SizeBytes   int64     `json:"size_bytes"`
	ModifiedAt  time.Time `json:"modified_at"`
	Description string    `json:"description"`
}

func GenerateDocumentIndex(store *Store, run *Run) (DocumentIndex, error) {
	index := DocumentIndex{RunID: run.RunID, GeneratedAt: time.Now().UTC()}
	err := filepath.WalkDir(run.RunDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "workspace" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(run.RunDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "artifacts/document-index.json" || rel == "artifacts/document-index.yaml" {
			return nil
		}
		kind := documentKind(rel)
		if kind == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		index.Documents = append(index.Documents, DocumentIndexEntry{
			Path:        rel,
			Kind:        kind,
			SizeBytes:   info.Size(),
			ModifiedAt:  info.ModTime().UTC(),
			Description: documentDescription(rel, kind),
		})
		return nil
	})
	if err != nil {
		return DocumentIndex{}, err
	}
	sort.Slice(index.Documents, func(i, j int) bool {
		return index.Documents[i].Path < index.Documents[j].Path
	})
	if err := store.SaveDocumentIndex(run, index); err != nil {
		return DocumentIndex{}, err
	}
	return index, nil
}

func (s *Store) SaveDocumentIndex(run *Run, index DocumentIndex) error {
	if err := writeJSON(filepath.Join(run.RunDir, "artifacts", "document-index.json"), index); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(run.RunDir, "artifacts", "document-index.yaml"), []byte(renderDocumentIndexYAML(index)), 0644)
}

func renderDocumentIndexYAML(index DocumentIndex) string {
	var b strings.Builder
	b.WriteString("document_index:\n")
	fmt.Fprintf(&b, "  run_id: %s\n", yamlQuote(index.RunID))
	fmt.Fprintf(&b, "  generated_at: %s\n", yamlQuote(index.GeneratedAt.Format(time.RFC3339)))
	b.WriteString("  documents:\n")
	for _, doc := range index.Documents {
		fmt.Fprintf(&b, "    - path: %s\n", yamlQuote(doc.Path))
		fmt.Fprintf(&b, "      kind: %s\n", yamlQuote(doc.Kind))
		fmt.Fprintf(&b, "      size_bytes: %d\n", doc.SizeBytes)
		fmt.Fprintf(&b, "      modified_at: %s\n", yamlQuote(doc.ModifiedAt.Format(time.RFC3339)))
		fmt.Fprintf(&b, "      description: %s\n", yamlQuote(doc.Description))
	}
	return b.String()
}

func documentKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".jsonl":
		return "jsonl"
	case ".patch":
		return "patch"
	case ".log":
		return "log"
	default:
		return ""
	}
}

func documentDescription(path, kind string) string {
	switch {
	case path == "order.md":
		return "Original Captain order."
	case path == "run.yaml":
		return "Human-readable run state."
	case strings.Contains(path, "task-graph."):
		return "Task graph or current graph pointer."
	case strings.Contains(path, "worker-input"):
		return "Worker input envelope."
	case strings.Contains(path, "worker-result"):
		return "Worker result envelope."
	case strings.Contains(path, "context-package"):
		return "Context package record."
	case strings.Contains(path, "gate-results"):
		return "Gate result evidence."
	case strings.Contains(path, "final-delivery-report"):
		return "Final delivery report."
	case strings.Contains(path, "run-evaluation"):
		return "Run evaluation record."
	case strings.Contains(path, "task-evaluations"):
		return "Task evaluation records."
	case strings.Contains(path, "logbook"):
		return "Append-only logbook record."
	case kind == "patch":
		return "Captured workspace patch artifact."
	default:
		return "Run document artifact."
	}
}
