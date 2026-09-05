package quadlet

import (
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func unit(body string) []descriptor.File {
	return []descriptor.File{{Path: "etc/containers/systemd/web.container", Data: []byte(body)}}
}

func TestRequiresNamesTheSecretAUnitAsksFor(t *testing.T) {
	got := NewReader().Requires(unit("[Container]\nSecret=db-password,type=env,target=DB_PASSWORD\n"))
	if want := "db-password"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestRequiresReadsASecretGivenByNameAlone(t *testing.T) {
	got := NewReader().Requires(unit("[Container]\nSecret=api-key\n"))
	if want := "api-key"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestRequiresNamesEachSecretOnce(t *testing.T) {
	files := append(unit("[Container]\nSecret=shared,type=env\n"),
		descriptor.File{Path: "etc/containers/systemd/db.container", Data: []byte("[Container]\nSecret=shared\n")})
	if got := NewReader().Requires(files); len(got) != 1 {
		t.Errorf("got %v, want one name", got)
	}
}

func TestRequiresSkipsACommentedSecret(t *testing.T) {
	if got := NewReader().Requires(unit("[Container]\n# Secret=old-one\n")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestRequiresReturnsNothingWhenNoUnitAsksForOne(t *testing.T) {
	if got := NewReader().Requires(unit("[Container]\nImage=x\n")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}
