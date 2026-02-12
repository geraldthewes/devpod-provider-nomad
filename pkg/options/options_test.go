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
		CSIVaultPath: "secret/data/ceph/csi",
		VaultAddr:    "https://vault.example.com:8200",
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

func TestValidateGPU_Disabled(t *testing.T) {
	opts := &Options{
		GPUEnabled: false,
	}

	err := opts.ValidateGPU()
	if err != nil {
		t.Errorf("Expected no error when GPU is disabled, got: %v", err)
	}
}

func TestValidateGPU_ValidConfig(t *testing.T) {
	opts := &Options{
		GPUEnabled:           true,
		GPUCount:             1,
		GPUComputeCapability: "7.5",
	}

	err := opts.ValidateGPU()
	if err != nil {
		t.Errorf("Expected no error for valid GPU config, got: %v", err)
	}
}

func TestValidateGPU_ValidConfigNoCapability(t *testing.T) {
	opts := &Options{
		GPUEnabled:           true,
		GPUCount:             2,
		GPUComputeCapability: "",
	}

	err := opts.ValidateGPU()
	if err != nil {
		t.Errorf("Expected no error for valid GPU config without capability, got: %v", err)
	}
}

func TestValidateGPU_InvalidComputeCapability_SingleNumber(t *testing.T) {
	opts := &Options{
		GPUEnabled:           true,
		GPUCount:             1,
		GPUComputeCapability: "75",
	}

	err := opts.ValidateGPU()
	if err == nil {
		t.Error("Expected error for invalid compute capability format (single number)")
	}
}

func TestValidateGPU_InvalidComputeCapability_NonNumeric(t *testing.T) {
	opts := &Options{
		GPUEnabled:           true,
		GPUCount:             1,
		GPUComputeCapability: "seven.five",
	}

	err := opts.ValidateGPU()
	if err == nil {
		t.Error("Expected error for non-numeric compute capability")
	}
}

func TestValidateGPU_InvalidComputeCapability_ThreeParts(t *testing.T) {
	opts := &Options{
		GPUEnabled:           true,
		GPUCount:             1,
		GPUComputeCapability: "7.5.0",
	}

	err := opts.ValidateGPU()
	if err == nil {
		t.Error("Expected error for compute capability with three parts")
	}
}

func TestValidateGPU_InvalidCount(t *testing.T) {
	opts := &Options{
		GPUEnabled: true,
		GPUCount:   0,
	}

	err := opts.ValidateGPU()
	if err == nil {
		t.Error("Expected error for GPU count of 0")
	}
}

func TestDefaultOptions_GPU(t *testing.T) {
	// Save current environment
	origGPU := os.Getenv("NOMAD_GPU")
	origGPUCount := os.Getenv("NOMAD_GPU_COUNT")
	origGPUCapability := os.Getenv("NOMAD_GPU_COMPUTE_CAPABILITY")

	// Clean environment for test
	os.Unsetenv("NOMAD_GPU")
	os.Unsetenv("NOMAD_GPU_COUNT")
	os.Unsetenv("NOMAD_GPU_COMPUTE_CAPABILITY")

	// Restore environment after test
	defer func() {
		if origGPU != "" {
			os.Setenv("NOMAD_GPU", origGPU)
		} else {
			os.Unsetenv("NOMAD_GPU")
		}
		if origGPUCount != "" {
			os.Setenv("NOMAD_GPU_COUNT", origGPUCount)
		} else {
			os.Unsetenv("NOMAD_GPU_COUNT")
		}
		if origGPUCapability != "" {
			os.Setenv("NOMAD_GPU_COMPUTE_CAPABILITY", origGPUCapability)
		} else {
			os.Unsetenv("NOMAD_GPU_COMPUTE_CAPABILITY")
		}
	}()

	opts, err := DefaultOptions()
	if err != nil {
		t.Fatalf("DefaultOptions failed: %v", err)
	}

	if opts.GPUEnabled != false {
		t.Errorf("Expected default GPU enabled to be false, got %v", opts.GPUEnabled)
	}

	if opts.GPUCount != defaultGPUCount {
		t.Errorf("Expected default GPU count %d, got %d", defaultGPUCount, opts.GPUCount)
	}

	if opts.GPUComputeCapability != "" {
		t.Errorf("Expected default GPU compute capability to be empty, got %s", opts.GPUComputeCapability)
	}
}

func TestDefaultOptions_GPUEnabled(t *testing.T) {
	// Save current environment
	origGPU := os.Getenv("NOMAD_GPU")
	origGPUCount := os.Getenv("NOMAD_GPU_COUNT")

	// Set environment for test
	os.Setenv("NOMAD_GPU", "true")
	os.Setenv("NOMAD_GPU_COUNT", "2")

	// Restore environment after test
	defer func() {
		if origGPU != "" {
			os.Setenv("NOMAD_GPU", origGPU)
		} else {
			os.Unsetenv("NOMAD_GPU")
		}
		if origGPUCount != "" {
			os.Setenv("NOMAD_GPU_COUNT", origGPUCount)
		} else {
			os.Unsetenv("NOMAD_GPU_COUNT")
		}
	}()

	opts, err := DefaultOptions()
	if err != nil {
		t.Fatalf("DefaultOptions failed: %v", err)
	}

	if opts.GPUEnabled != true {
		t.Errorf("Expected GPU enabled to be true, got %v", opts.GPUEnabled)
	}

	if opts.GPUCount != 2 {
		t.Errorf("Expected GPU count 2, got %d", opts.GPUCount)
	}
}
