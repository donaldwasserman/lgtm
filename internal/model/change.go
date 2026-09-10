package model

// ChangeKind classifies how a node changed between the base and head trees.
type ChangeKind string

const (
	// KindInsert marks a node present only in the head tree.
	KindInsert ChangeKind = "insert"
	// KindDelete marks a node present only in the base tree.
	KindDelete ChangeKind = "delete"
	// KindModify marks a node present in both trees whose structure or
	// descendant content changed.
	KindModify ChangeKind = "modify"
)

// FileChange describes how one file changed and the changed nodes within it.
type FileChange struct {
	Path    string        `json:"path"`
	Kind    ChangeKind    `json:"kind"` // of the file itself (insert/delete/modify)
	Changes []*NodeChange `json:"changes"`
}

// NodeChange is a single changed node with classification and depth.
type NodeChange struct {
	Kind  ChangeKind `json:"kind"`
	Type  string     `json:"type"`
	Name  string     `json:"name,omitempty"`
	Depth int        `json:"depth"`
}
