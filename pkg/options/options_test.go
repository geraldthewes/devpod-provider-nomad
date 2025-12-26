package options

import (
	"os"
	"testing"
)

func TestValidateCSI_EphemeralMode(t *testing.T) {
	opts := &Options{
		StorageMode: StorageModeEphemeral,
	}

	err := opts.ValidateCSI()
	if err != nil {
		t.Errorf("Expected no error for ephemeral mode, got: %v", err)
	}
}

func TestValidateCSI_PersistentModeWithClusterID(t *testing.T) {
	opts := &Options{
		StorageMode:  StorageModePersistent,
		CSIClusterID: "test-cluster-id",
		CSIPluginID:  "ceph-csi",
	}

	err := opts.ValidateCSI()
	if err != nil {
		t.Errorf("Expected no error for persistent mode with cluster ID, got: %v", err)
	}
}

func TestValidateCSI_PersistentModeMissingClusterID(t *testing.T) {
	opts := &Options{
		StorageMode: StorageModePersistent,
		CSIPluginID: "ceph-csi",
	}

	err := opts.ValidateCSI()
	if err == nil {
		t.Error("Expected error for persistent mode without cluster ID")
	}
}

func TestValidateCSI_PersistentModeMissingPluginID(t *testing.T) {
	opts := &Options{
		StorageMode:  StorageModePersistent,
		CSIClusterID: "test-cluster-id",
	}

	err := opts.ValidateCSI()
	if err == nil {
		t.Error("Expected error for persistent mode without plugin ID")
	}
}

func TestValidateCSI_InvalidStorageMode(t *testing.T) {
	opts := &Options{
		StorageMode: "invalid",
	}

	err := opts.ValidateCSI()
	if err == nil {
		t.Error("Expected error for invalid storage mode")
	}
}

func TestGetVolumeID(t *testing.T) {
	opts := &Options{
		JobId: "my-workspace",
	}

	volumeID := opts.GetVolumeID()
	expected := "devpod-my-workspace"

	if volumeID != expected {
		t.Errorf("Expected volume ID %s, got %s", expected, volumeID)
	}
}

func TestDefaultOptions_StorageMode(t *testing.T) {
	// Save current environment
	origStorageMode := os.Getenv("NOMAD_STORAGE_MODE")
	origPluginID := os.Getenv("NOMAD_CSI_PLUGIN_ID")
	origClusterID := os.Getenv("NOMAD_CSI_CLUSTER_ID")
	origPool := os.Getenv("NOMAD_CSI_POOL")

	// Clean environment for test
	os.Unsetenv("NOMAD_STORAGE_MODE")
	os.Unsetenv("NOMAD_CSI_PLUGIN_ID")
	os.Unsetenv("NOMAD_CSI_CLUSTER_ID")
	os.Unsetenv("NOMAD_CSI_POOL")

	// Restore environment after test
	defer func() {
		if origStorageMode != "" {
			os.Setenv("NOMAD_STORAGE_MODE", origStorageMode)
		}
		if origPluginID != "" {
			os.Setenv("NOMAD_CSI_PLUGIN_ID", origPluginID)
		}
		if origClusterID != "" {
			os.Setenv("NOMAD_CSI_CLUSTER_ID", origClusterID)
		}
		if origPool != "" {
			os.Setenv("NOMAD_CSI_POOL", origPool)
		}
	}()

	opts, err := DefaultOptions()
	if err != nil {
		t.Fatalf("DefaultOptions failed: %v", err)
	}

	if opts.StorageMode != defaultStorageMode {
		t.Errorf("Expected default storage mode %s, got %s", defaultStorageMode, opts.StorageMode)
	}

	if opts.CSIPluginID != defaultCSIPluginID {
		t.Errorf("Expected default CSI plugin ID %s, got %s", defaultCSIPluginID, opts.CSIPluginID)
	}

	if opts.CSIPool != defaultCSIPool {
		t.Errorf("Expected default CSI pool %s, got %s", defaultCSIPool, opts.CSIPool)
	}
}
