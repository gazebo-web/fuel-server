package aibuilder

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/jinzhu/gorm"
)

// Files a user attaches to a chat message: project files
// (bundles/projects/assets.go), named by id or name. The turn's user
// message carries their pictures; later turns only mention them, and the
// model looks again with asset_view.

// maxAttachments caps the files on one message.
const maxAttachments = 10

// Attachment is a file attached to a message, as stored with it.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Summary is e.g. "2000×1000 px", for the model's note.
	Summary string `json:"summary,omitempty"`
}

// Attachments is stored as JSON.
type Attachments []Attachment

// Value stores Attachments as JSON.
func (a Attachments) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	b, err := json.Marshal(a)
	return string(b), err
}

// Scan reads Attachments from JSON.
func (a *Attachments) Scan(v any) error {
	var b []byte
	switch x := v.(type) {
	case nil:
		*a = nil
		return nil
	case []byte:
		b = x
	case string:
		b = []byte(x)
	default:
		return fmt.Errorf("attachments: unexpected %T", v)
	}
	if len(b) == 0 {
		*a = nil
		return nil
	}
	return json.Unmarshal(b, a)
}

// UserInput is a user's message: their text, the files they attached,
// and those files' pictures for this turn.
type UserInput struct {
	Text        string
	Attachments Attachments
	Images      []llm.Image
}

// withNote is the text the model reads: the message, then the attached
// files.
func withNote(text string, atts Attachments) string {
	if len(atts) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n[Attached project files; look at them again any time with asset_view:")
	for _, a := range atts {
		fmt.Fprintf(&b, "\n- %s (id %s, %s", a.Name, a.ID, a.Kind)
		if a.Summary != "" {
			b.WriteString(", " + a.Summary)
		}
		b.WriteString(")")
	}
	b.WriteString("]")
	return b.String()
}

// resolveAttachments loads the attached files of project for user, with
// the picture of each one that has one.
func resolveAttachments(db *gorm.DB, project string, user *users.User, text string, refs []string) (UserInput, error) {
	in := UserInput{Text: text}
	if len(refs) > maxAttachments {
		return in, fmt.Errorf("attach at most %d files to a message", maxAttachments)
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		p, a, em := projectService.GetAsset(db, project, ref, user)
		if em != nil {
			return in, fmt.Errorf("attached file %q: %s", ref, em.Msg)
		}
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		att := Attachment{ID: a.ID, Name: a.Name, Kind: a.Kind}
		if w, ok := a.Info["width"]; ok {
			att.Summary = fmt.Sprintf("%v×%v px", w, a.Info["height"])
		}
		if a.HasView("ai.jpg") {
			b, em := projects.ReadAssetView(p, a, "ai.jpg")
			if em != nil {
				return in, fmt.Errorf("attached file %q: %s", a.Name, em.Msg)
			}
			in.Images = append(in.Images, llm.Image{MimeType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(b)})
		}
		in.Attachments = append(in.Attachments, att)
	}
	return in, nil
}
