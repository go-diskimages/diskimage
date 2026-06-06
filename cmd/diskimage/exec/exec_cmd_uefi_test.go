package exec

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	filesystem_uefi "github.com/go-filesystems/uefi"
)

// fakeUEFIStore is an in-memory implementation of uefiStoreIface for tests.
type fakeUEFIStore struct {
	vars      []filesystem_uefi.Variable
	openErr   error
	setErr    error
	deleteErr error
}

func (f *fakeUEFIStore) Close() error { return nil }

func (f *fakeUEFIStore) List() []filesystem_uefi.Variable {
	cp := make([]filesystem_uefi.Variable, len(f.vars))
	copy(cp, f.vars)
	return cp
}

func (f *fakeUEFIStore) Get(name string, _ filesystem_uefi.GUID) (filesystem_uefi.Variable, error) {
	for _, v := range f.vars {
		if v.Name == name {
			return v, nil
		}
	}
	return filesystem_uefi.Variable{}, fmt.Errorf("variable %q not found", name)
}

func (f *fakeUEFIStore) Set(v filesystem_uefi.Variable) error {
	if f.setErr != nil {
		return f.setErr
	}
	for i, existing := range f.vars {
		if existing.Name == v.Name {
			f.vars[i] = v
			return nil
		}
	}
	f.vars = append(f.vars, v)
	return nil
}

func (f *fakeUEFIStore) Delete(name string, _ filesystem_uefi.GUID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i, v := range f.vars {
		if v.Name == name {
			f.vars = append(f.vars[:i], f.vars[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("variable %q not found", name)
}

// injectUEFIStore returns the previous openUEFIStoreFunc after replacing it
// with one that returns store. The returned restore func must be deferred.
func injectUEFIStore(store *fakeUEFIStore) (restore func()) {
	prev := SetOpenUEFIStoreFunc(func(_ string) (uefiStoreIface, error) {
		if store.openErr != nil {
			return nil, store.openErr
		}
		return store, nil
	})
	return func() { SetOpenUEFIStoreFunc(prev) }
}

// -- uefi list ----------------------------------------------------------------

func TestExecUefiList_Empty(t *testing.T) {
	defer injectUEFIStore(&fakeUEFIStore{})()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"uefi list", "--file", "fake.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "(no variables)") {
		t.Fatalf("expected '(no variables)', got: %q", out.String())
	}
}

func TestExecUefiList_WithVars(t *testing.T) {
	store := &fakeUEFIStore{vars: []filesystem_uefi.Variable{
		{Name: "BootOrder", GUID: filesystem_uefi.DefaultNamespaceGUID, Attributes: filesystem_uefi.AttrNonVolatile | filesystem_uefi.AttrBootServiceAccess | filesystem_uefi.AttrRuntimeAccess, Data: []byte{0x01, 0x00}},
		{Name: "Lang", GUID: filesystem_uefi.DefaultNamespaceGUID, Attributes: filesystem_uefi.AttrBootServiceAccess, Data: []byte("en")},
	}}
	defer injectUEFIStore(store)()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"uefi list", "--file", "fake.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "BootOrder") || !strings.Contains(s, "Lang") {
		t.Fatalf("expected variable names in output: %q", s)
	}
	if !strings.Contains(s, "NV+BS+RT") {
		t.Fatalf("expected attribute flags in output: %q", s)
	}
}

func TestExecUefiList_OpenError(t *testing.T) {
	defer injectUEFIStore(&fakeUEFIStore{openErr: fmt.Errorf("no such file")})()
	cmd := Command()
	cmd.SetArgs([]string{"uefi list", "--file", "missing.fd"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("expected open error, got: %v", err)
	}
}

func TestExecUefiList_FileRequired(t *testing.T) {
	cmd := Command()
	cmd.SetArgs([]string{"uefi list"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected '--file is required' error, got: %v", err)
	}
}

// -- uefi get -----------------------------------------------------------------

func TestExecUefiGet_Found(t *testing.T) {
	store := &fakeUEFIStore{vars: []filesystem_uefi.Variable{
		{Name: "BootOrder", GUID: filesystem_uefi.DefaultNamespaceGUID, Data: []byte{0x01, 0x02, 0x03}},
	}}
	defer injectUEFIStore(store)()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"uefi get BootOrder", "--file", "fake.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "010203") {
		t.Fatalf("expected hex 010203 in output: %q", out.String())
	}
}

func TestExecUefiGet_NotFound(t *testing.T) {
	defer injectUEFIStore(&fakeUEFIStore{})()
	cmd := Command()
	cmd.SetArgs([]string{"uefi get Missing", "--file", "fake.fd"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestExecUefiGet_FileRequired(t *testing.T) {
	cmd := Command()
	cmd.SetArgs([]string{"uefi get BootOrder"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected '--file is required' error, got: %v", err)
	}
}

// -- uefi set -----------------------------------------------------------------

func TestExecUefiSet_Success(t *testing.T) {
	store := &fakeUEFIStore{}
	defer injectUEFIStore(store)()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"uefi set BootOrder --data 0100", "--file", "fake.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "set BootOrder (2 bytes)") {
		t.Fatalf("expected confirmation in output: %q", out.String())
	}
	if len(store.vars) != 1 || store.vars[0].Name != "BootOrder" {
		t.Fatalf("expected var to be stored, got: %v", store.vars)
	}
}

func TestExecUefiSet_InvalidHex(t *testing.T) {
	defer injectUEFIStore(&fakeUEFIStore{})()
	cmd := Command()
	cmd.SetArgs([]string{"uefi set BootOrder --data ZZZZ", "--file", "fake.fd"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid hex") {
		t.Fatalf("expected invalid hex error, got: %v", err)
	}
}

func TestExecUefiSet_SetError(t *testing.T) {
	store := &fakeUEFIStore{setErr: fmt.Errorf("store full")}
	defer injectUEFIStore(store)()
	cmd := Command()
	cmd.SetArgs([]string{"uefi set Foo --data 01", "--file", "fake.fd"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "store full") {
		t.Fatalf("expected store error, got: %v", err)
	}
}

// -- uefi delete --------------------------------------------------------------

func TestExecUefiDelete_Success(t *testing.T) {
	store := &fakeUEFIStore{vars: []filesystem_uefi.Variable{
		{Name: "BootOrder", GUID: filesystem_uefi.DefaultNamespaceGUID, Data: []byte{0x01}},
	}}
	defer injectUEFIStore(store)()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"uefi delete BootOrder", "--file", "fake.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "deleted BootOrder") {
		t.Fatalf("expected confirmation in output: %q", out.String())
	}
	if len(store.vars) != 0 {
		t.Fatalf("expected var to be removed")
	}
}

func TestExecUefiDelete_NotFound(t *testing.T) {
	defer injectUEFIStore(&fakeUEFIStore{})()
	cmd := Command()
	cmd.SetArgs([]string{"uefi delete Missing", "--file", "fake.fd"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

// -- formatUEFIAttrs ----------------------------------------------------------

func TestFormatUEFIAttrs(t *testing.T) {
	cases := []struct {
		attrs filesystem_uefi.Attributes
		want  string
	}{
		{filesystem_uefi.AttrNonVolatile | filesystem_uefi.AttrBootServiceAccess | filesystem_uefi.AttrRuntimeAccess, "NV+BS+RT"},
		{filesystem_uefi.AttrBootServiceAccess, "BS"},
		{0, "-"},
	}
	for _, c := range cases {
		got := formatUEFIAttrs(c.attrs)
		if got != c.want {
			t.Errorf("formatUEFIAttrs(%d) = %q, want %q", c.attrs, got, c.want)
		}
	}
}
