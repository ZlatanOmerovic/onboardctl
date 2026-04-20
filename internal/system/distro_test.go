package system

import (
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantID  string
		wantCN  string
		wantFam string
	}{
		{
			name: "debian trixie",
			in: `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
VERSION_CODENAME=trixie
ID=debian
HOME_URL="https://www.debian.org/"
`,
			wantID: "debian", wantCN: "trixie", wantFam: "debian",
		},
		{
			name: "ubuntu noble",
			in: `PRETTY_NAME="Ubuntu 24.04 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION="24.04 LTS (Noble Numbat)"
VERSION_CODENAME=noble
ID=ubuntu
ID_LIKE=debian
`,
			wantID: "ubuntu", wantCN: "noble", wantFam: "debian",
		},
		{
			name: "linux mint (ubuntu-based)",
			in: `NAME="Linux Mint"
VERSION_ID="22"
VERSION="22 (Wilma)"
VERSION_CODENAME=wilma
ID=linuxmint
ID_LIKE="ubuntu debian"
`,
			wantID: "linuxmint", wantCN: "wilma", wantFam: "debian",
		},
		{
			name: "fedora (not in family)",
			in: `NAME=Fedora
VERSION_ID=40
ID=fedora
`,
			wantID: "fedora", wantCN: "", wantFam: "fedora",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parseOSRelease(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("parseOSRelease error: %v", err)
			}
			if d.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", d.ID, tt.wantID)
			}
			if d.Codename != tt.wantCN {
				t.Errorf("Codename = %q, want %q", d.Codename, tt.wantCN)
			}
			if d.Family != tt.wantFam {
				t.Errorf("Family = %q, want %q", d.Family, tt.wantFam)
			}
		})
	}
}

func TestInDebianFamily(t *testing.T) {
	cases := []struct {
		in   Distro
		want bool
	}{
		{Distro{ID: "debian"}, true},
		{Distro{ID: "ubuntu", IDLike: []string{"debian"}}, true},
		{Distro{ID: "linuxmint", IDLike: []string{"ubuntu", "debian"}}, true},
		{Distro{ID: "fedora"}, false},
		{Distro{ID: "arch"}, false},
	}
	for _, c := range cases {
		got := Distro{ID: c.in.ID, IDLike: c.in.IDLike, Family: resolveFamily(c.in)}.InDebianFamily()
		if got != c.want {
			t.Errorf("InDebianFamily(%+v) = %v, want %v", c.in, got, c.want)
		}
	}
}
