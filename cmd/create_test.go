package cmd

import (
	"testing"

	opts "github.com/briancain/devpod-provider-nomad/pkg/options"
)

func TestBuildGPUDeviceRequest_NameAndCount(t *testing.T) {
	options := &opts.Options{
		GPUCount: 2,
	}

	device := buildGPUDeviceRequest(options)

	if device.Name != "nvidia/gpu" {
		t.Errorf("Expected device name 'nvidia/gpu', got %q", device.Name)
	}
	if device.Count == nil || *device.Count != 2 {
		t.Errorf("Expected device count 2, got %v", device.Count)
	}
}

func TestBuildGPUDeviceRequest_NoConstraints(t *testing.T) {
	options := &opts.Options{
		GPUCount:             1,
		GPUComputeCapability: "7.5",
	}

	device := buildGPUDeviceRequest(options)

	if len(device.Constraints) != 0 {
		t.Errorf("Expected no device constraints, got %d", len(device.Constraints))
	}
}

func TestBuildGPUJobConstraints_NoComputeCapability(t *testing.T) {
	options := &opts.Options{
		GPUComputeCapability: "",
	}

	constraints := buildGPUJobConstraints(options)

	if len(constraints) != 2 {
		t.Fatalf("Expected 2 constraints, got %d", len(constraints))
	}
	if constraints[0].LTarget != "${attr.cpu.arch}" {
		t.Errorf("Expected first constraint LTarget '${attr.cpu.arch}', got %q", constraints[0].LTarget)
	}
	if constraints[0].Operand != "=" || constraints[0].RTarget != "amd64" {
		t.Errorf("Unexpected first constraint: operand=%q rtarget=%q", constraints[0].Operand, constraints[0].RTarget)
	}
	if constraints[1].LTarget != "${meta.gpu-dedicated}" {
		t.Errorf("Expected second constraint LTarget '${meta.gpu-dedicated}', got %q", constraints[1].LTarget)
	}
	if constraints[1].Operand != "!=" || constraints[1].RTarget != "true" {
		t.Errorf("Unexpected second constraint: operand=%q rtarget=%q", constraints[1].Operand, constraints[1].RTarget)
	}
}

func TestBuildGPUJobConstraints_WithComputeCapability(t *testing.T) {
	options := &opts.Options{
		GPUComputeCapability: "7.5",
	}

	constraints := buildGPUJobConstraints(options)

	if len(constraints) != 3 {
		t.Fatalf("Expected 3 constraints, got %d", len(constraints))
	}
	// First two are arch and gpu-dedicated (verified in other test)
	cc := constraints[2]
	if cc.LTarget != "${meta.gpu_compute_capability}" {
		t.Errorf("Expected compute capability LTarget '${meta.gpu_compute_capability}', got %q", cc.LTarget)
	}
	if cc.Operand != ">=" {
		t.Errorf("Expected operand '>=', got %q", cc.Operand)
	}
	if cc.RTarget != "7.5" {
		t.Errorf("Expected RTarget '7.5', got %q", cc.RTarget)
	}
}
