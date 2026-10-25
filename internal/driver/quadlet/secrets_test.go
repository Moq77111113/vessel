package quadlet

import (
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

func unit(body string) []delivery.File {
	return []delivery.File{{Path: "etc/containers/systemd/web.container", Data: []byte(body)}}
}

func TestReadsNamesTheSecretAUnitAsksFor(t *testing.T) {
	got := reads(unit("[Container]\nSecret=db-password,type=env,target=DB_PASSWORD\n"))
	if want := "db-password"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestReadsReadsASecretWrittenWithSpacesAroundTheEquals(t *testing.T) {
	got := reads(unit("[Container]\nSecret = db-password,type=env,target=DB_PASSWORD\n"))
	if want := "db-password"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestReadsReadsASecretGivenByNameAlone(t *testing.T) {
	got := reads(unit("[Container]\nSecret=api-key\n"))
	if want := "api-key"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestReadsNamesEachSecretOnce(t *testing.T) {
	files := append(unit("[Container]\nSecret=shared,type=env\n"),
		delivery.File{Path: "etc/containers/systemd/db.container", Data: []byte("[Container]\nSecret=shared\n")})
	if got := reads(files); len(got) != 1 {
		t.Errorf("got %v, want one name", got)
	}
}

func TestReadsSkipsACommentedSecret(t *testing.T) {
	if got := reads(unit("[Container]\n# Secret=old-one\n")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestReadsReturnsNothingWhenNoUnitAsksForOne(t *testing.T) {
	if got := reads(unit("[Container]\nImage=x\n")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestReadsLeavesOutAFileThatIsNotInTheUnitDirectory(t *testing.T) {
	files := []delivery.File{{Path: "etc/acme/backup.conf", Data: []byte("Secret=not-a-unit\n")}}
	if got := reads(files); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}
