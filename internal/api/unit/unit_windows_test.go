package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ushineko/hayami/internal/nethelper"
)

// R25. An instance is the template's name, an underscore and Windows' own
// number; nothing else with the name in it is one.
func TestAnInstanceIsTheTemplatesNameAndANumber(t *testing.T) {
	assert.True(t, isInstance("hayami-api_5a3f1"))
	assert.False(t, isInstance("hayami-api"))
	assert.False(t, isInstance("hayami-api_"))
	assert.False(t, isInstance("hayami-apix_1"))
	assert.False(t, isInstance("hayami-network"))
}

// R6. The template's program is in Program Files, the known folder's, never
// the install under %LOCALAPPDATA% that the person can write.
func TestTheProgramIsInProgramFiles(t *testing.T) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(pf, "hayami", "hayami-api.exe"), Program())
	t.Setenv("ProgramFiles", t.TempDir())
	assert.Equal(t, filepath.Join(pf, "hayami", "hayami-api.exe"), Program(), "the environment does not move it")
}

/*
An update that fails after stopping the instances starts them again on the
copy there, rather than leaving the account without its service until the
next sign-in (the adversarial review). Run elevated, with
HAYAMI_SCM_INSTANCE_TEST=1, on the Windows VM with the service installed and
an instance running.
*/
func TestAFailedUpdateLeavesTheServiceRunning(t *testing.T) {
	if os.Getenv("HAYAMI_SCM_INSTANCE_TEST") != "1" || !nethelper.Elevated() {
		t.Skip("stops the installed service: run elevated with HAYAMI_SCM_INSTANCE_TEST=1, on the Windows VM")
	}
	require.True(t, Installed())
	running := func() []string {
		m, err := mgr.Connect()
		require.NoError(t, err)
		defer func() { _ = m.Disconnect() }()
		names, err := m.ListServices()
		require.NoError(t, err)
		var up []string
		for _, n := range names {
			if !isInstance(n) {
				continue
			}
			s, err := m.OpenService(n)
			require.NoError(t, err)
			if st, err := s.Query(); err == nil && st.State == 4 /* SERVICE_RUNNING */ {
				up = append(up, n)
			}
			_ = s.Close()
		}
		return up
	}
	before := running()
	require.NotEmpty(t, before, "an instance must be running")

	err := Install(context.Background(), filepath.Join(t.TempDir(), "absent.exe"))
	require.Error(t, err)
	assert.Eventually(t, func() bool { return len(running()) == len(before) }, 15*time.Second, 250*time.Millisecond,
		"the instances stopped for the update run again")
}

/*
R6, R25, on a machine where the test may register services: the template is
registered as a per-user service, automatic, running a copy in Program Files
with `serve`, with its recovery actions; installed again it is updated in
place; removed, it and its program are gone. Run elevated, with
HAYAMI_SCM_TEST=1, on the Windows VM. It registers the real template, so not
on a machine whose hayami has the service installed.
*/
func TestTheTemplateIsRegisteredAndRemoved(t *testing.T) {
	if os.Getenv("HAYAMI_SCM_TEST") != "1" || !nethelper.Elevated() {
		t.Skip("registers a service: run elevated with HAYAMI_SCM_TEST=1, on the Windows VM")
	}
	require.False(t, Installed(), "this machine has the service installed already")
	self, err := os.Executable()
	require.NoError(t, err)
	ctx := context.Background()
	t.Cleanup(func() { _ = Remove(ctx) })

	for range 2 {
		require.NoError(t, Install(ctx, self))
		require.True(t, Installed())

		m, err := mgr.Connect()
		require.NoError(t, err)
		s, err := m.OpenService(TemplateName)
		require.NoError(t, err)
		config, err := s.Config()
		require.NoError(t, err)
		assert.Equal(t, uint32(serviceUserOwnProcess), config.ServiceType&serviceUserOwnProcess)
		assert.Equal(t, uint32(mgr.StartAutomatic), config.StartType)
		assert.True(t, strings.EqualFold(config.BinaryPathName, `"`+Program()+`" serve`) ||
			strings.EqualFold(config.BinaryPathName, Program()+" serve"), config.BinaryPathName)
		actions, err := s.RecoveryActions()
		require.NoError(t, err)
		require.Len(t, actions, 3)
		assert.Equal(t, mgr.ServiceRestart, actions[0].Type)
		assert.Equal(t, 5*time.Second, actions[0].Delay)
		assert.Equal(t, mgr.ServiceRestart, actions[1].Type)
		assert.Equal(t, mgr.NoAction, actions[2].Type)
		_ = s.Close()
		_ = m.Disconnect()
		_, err = os.Stat(Program())
		require.NoError(t, err)
	}

	require.NoError(t, Remove(ctx))
	assert.False(t, Installed())
	_, err = os.Stat(Program())
	assert.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, Remove(ctx), "removing what is not there is not an error")
}
