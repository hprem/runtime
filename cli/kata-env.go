// Copyright (c) 2017-2019 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"errors"
	"os"
	runtim "runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kata-containers/runtime/pkg/katautils"
	vc "github.com/kata-containers/runtime/virtcontainers"
	exp "github.com/kata-containers/runtime/virtcontainers/experimental"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
	vcUtils "github.com/kata-containers/runtime/virtcontainers/utils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

// Semantic version for the output of the command.
//
// XXX: Increment for every change to the output format
// (meaning any change to the EnvInfo type).
const formatVersion = "1.0.24"

// MetaInfo stores information on the format of the output itself
type MetaInfo struct {
	// output format version
	Version string
}

// KernelInfo stores kernel details
type KernelInfo struct {
	Path       string
	Parameters string
}

// InitrdInfo stores initrd image details
type InitrdInfo struct {
	Path string
}

// ImageInfo stores root filesystem image details
type ImageInfo struct {
	Path string
}

// CPUInfo stores host CPU details
type CPUInfo struct {
	Vendor string
	Model  string
}

// RuntimeConfigInfo stores runtime config details.
type RuntimeConfigInfo struct {
	Path string
}

// RuntimeInfo stores runtime details.
type RuntimeInfo struct {
	Version             RuntimeVersionInfo
	Config              RuntimeConfigInfo
	Debug               bool
	Trace               bool
	DisableGuestSeccomp bool
	DisableNewNetNs     bool
	SandboxCgroupOnly   bool
	Experimental        []exp.Feature
	Path                string
}

type VersionInfo struct {
	Semver string
	Major  uint64
	Minor  uint64
	Patch  uint64
	Commit string
}

// RuntimeVersionInfo stores details of the runtime version
type RuntimeVersionInfo struct {
	Version VersionInfo
	OCI     string
}

// HypervisorInfo stores hypervisor details
type HypervisorInfo struct {
	MachineType          string
	Version              string
	Path                 string
	BlockDeviceDriver    string
	EntropySource        string
	SharedFS             string
	VirtioFSDaemon       string
	Msize9p              uint32
	MemorySlots          uint32
	PCIeRootPort         uint32
	HotplugVFIOOnRootBus bool
	Debug                bool
	UseVSock             bool
}

// ProxyInfo stores proxy details
type ProxyInfo struct {
	Type    string
	Version VersionInfo
	Path    string
	Debug   bool
}

// ShimInfo stores shim details
type ShimInfo struct {
	Type    string
	Version VersionInfo
	Path    string
	Debug   bool
}

// AgentInfo stores agent details
type AgentInfo struct {
	Type      string
	Debug     bool
	Trace     bool
	TraceMode string
	TraceType string
}

// DistroInfo stores host operating system distribution details.
type DistroInfo struct {
	Name    string
	Version string
}

// HostInfo stores host details
type HostInfo struct {
	Kernel             string
	Architecture       string
	Distro             DistroInfo
	CPU                CPUInfo
	VMContainerCapable bool
	SupportVSocks      bool
}

// NetmonInfo stores netmon details
type NetmonInfo struct {
	Version VersionInfo
	Path    string
	Debug   bool
	Enable  bool
}

// EnvInfo collects all information that will be displayed by the
// env command.
//
// XXX: Any changes must be coupled with a change to formatVersion.
type EnvInfo struct {
	Meta       MetaInfo
	Runtime    RuntimeInfo
	Hypervisor HypervisorInfo
	Image      ImageInfo
	Kernel     KernelInfo
	Initrd     InitrdInfo
	Proxy      ProxyInfo
	Shim       ShimInfo
	Agent      AgentInfo
	Host       HostInfo
	Netmon     NetmonInfo
}

func getMetaInfo() MetaInfo {
	return MetaInfo{
		Version: formatVersion,
	}
}

func getRuntimeInfo(configFile string, config oci.RuntimeConfig) RuntimeInfo {
	runtimeVersionInfo := constructVersionInfo(version)
	runtimeVersionInfo.Commit = commit

	runtimeVersion := RuntimeVersionInfo{
		Version: runtimeVersionInfo,
		OCI:     specs.Version,
	}

	runtimeConfig := RuntimeConfigInfo{
		Path: configFile,
	}

	runtimePath, _ := os.Executable()

	return RuntimeInfo{
		Debug:               config.Debug,
		Trace:               config.Trace,
		Version:             runtimeVersion,
		Config:              runtimeConfig,
		Path:                runtimePath,
		DisableNewNetNs:     config.DisableNewNetNs,
		SandboxCgroupOnly:   config.SandboxCgroupOnly,
		Experimental:        config.Experimental,
		DisableGuestSeccomp: config.DisableGuestSeccomp,
	}
}

func getHostInfo() (HostInfo, error) {
	hostKernelVersion, err := getKernelVersion()
	if err != nil {
		return HostInfo{}, err
	}

	hostDistroName, hostDistroVersion, err := getDistroDetails()
	if err != nil {
		return HostInfo{}, err
	}

	cpuVendor, cpuModel, err := getCPUDetails()
	if err != nil {
		return HostInfo{}, err
	}

	hostVMContainerCapable := true
	if runtim.GOARCH == "ppc64le" {
		hostVMContainerCapable = false
	}

	details := vmContainerCapableDetails{
		cpuInfoFile:           procCPUInfo,
		requiredCPUFlags:      archRequiredCPUFlags,
		requiredCPUAttribs:    archRequiredCPUAttribs,
		requiredKernelModules: archRequiredKernelModules,
	}

	if err = hostIsVMContainerCapable(details); err != nil {
		hostVMContainerCapable = false
	}

	hostDistro := DistroInfo{
		Name:    hostDistroName,
		Version: hostDistroVersion,
	}

	hostCPU := CPUInfo{
		Vendor: cpuVendor,
		Model:  cpuModel,
	}

	host := HostInfo{
		Kernel:             hostKernelVersion,
		Architecture:       arch,
		Distro:             hostDistro,
		CPU:                hostCPU,
		VMContainerCapable: hostVMContainerCapable,
		SupportVSocks:      vcUtils.SupportsVsocks(),
	}

	return host, nil
}

func getProxyInfo(config oci.RuntimeConfig) ProxyInfo {
	if config.ProxyType == vc.NoProxyType {
		return ProxyInfo{Type: string(config.ProxyType)}
	}

	proxyConfig := config.ProxyConfig

	var proxyVersionInfo VersionInfo
	if version, err := getCommandVersion(proxyConfig.Path); err != nil {
		proxyVersionInfo = unknownVersionInfo
	} else {
		proxyVersionInfo = constructVersionInfo(version)
	}

	proxy := ProxyInfo{
		Type:    string(config.ProxyType),
		Version: proxyVersionInfo,
		Path:    proxyConfig.Path,
		Debug:   proxyConfig.Debug,
	}

	return proxy
}

func getNetmonInfo(config oci.RuntimeConfig) NetmonInfo {
	netmonConfig := config.NetmonConfig

	var netmonVersionInfo VersionInfo
	if version, err := getCommandVersion(netmonConfig.Path); err != nil {
		netmonVersionInfo = unknownVersionInfo
	} else {
		netmonVersionInfo = constructVersionInfo(version)
	}

	netmon := NetmonInfo{
		Version: netmonVersionInfo,
		Path:    netmonConfig.Path,
		Debug:   netmonConfig.Debug,
		Enable:  netmonConfig.Enable,
	}

	return netmon
}

func getCommandVersion(cmd string) (string, error) {
	return katautils.RunCommand([]string{cmd, "--version"})
}

func getShimInfo(config oci.RuntimeConfig) (ShimInfo, error) {
	shimConfig, ok := config.ShimConfig.(vc.ShimConfig)
	if !ok {
		return ShimInfo{}, errors.New("cannot determine shim config")
	}

	shimPath := shimConfig.Path

	var shimVersionInfo VersionInfo
	if version, err := getCommandVersion(shimConfig.Path); err != nil {
		shimVersionInfo = unknownVersionInfo
	} else {
		shimVersionInfo = constructVersionInfo(version)
	}

	shim := ShimInfo{
		Type:    string(config.ShimType),
		Version: shimVersionInfo,
		Path:    shimPath,
		Debug:   shimConfig.Debug,
	}

	return shim, nil
}

func getAgentInfo(config oci.RuntimeConfig) (AgentInfo, error) {
	agent := AgentInfo{
		Type: string(config.AgentType),
	}

	switch config.AgentType {
	case vc.KataContainersAgent:
		agentConfig, ok := config.AgentConfig.(vc.KataAgentConfig)
		if !ok {
			return AgentInfo{}, errors.New("cannot determine Kata agent config")
		}
		agent.Debug = agentConfig.Debug
		agent.Trace = agentConfig.Trace
		agent.TraceMode = agentConfig.TraceMode
		agent.TraceType = agentConfig.TraceType
	default:
		// Nothing useful to report for the other agent types
	}

	return agent, nil
}

func getHypervisorInfo(config oci.RuntimeConfig) HypervisorInfo {
	hypervisorPath := config.HypervisorConfig.HypervisorPath

	version, err := getCommandVersion(hypervisorPath)
	if err != nil {
		version = unknown
	}

	return HypervisorInfo{
		Debug:             config.HypervisorConfig.Debug,
		MachineType:       config.HypervisorConfig.HypervisorMachineType,
		Version:           version,
		Path:              hypervisorPath,
		BlockDeviceDriver: config.HypervisorConfig.BlockDeviceDriver,
		Msize9p:           config.HypervisorConfig.Msize9p,
		UseVSock:          config.HypervisorConfig.UseVSock,
		MemorySlots:       config.HypervisorConfig.MemSlots,
		EntropySource:     config.HypervisorConfig.EntropySource,
		SharedFS:          config.HypervisorConfig.SharedFS,
		VirtioFSDaemon:    config.HypervisorConfig.VirtioFSDaemon,

		HotplugVFIOOnRootBus: config.HypervisorConfig.HotplugVFIOOnRootBus,
		PCIeRootPort:         config.HypervisorConfig.PCIeRootPort,
	}
}

func getEnvInfo(configFile string, config oci.RuntimeConfig) (env EnvInfo, err error) {
	err = setCPUtype(config.HypervisorType)
	if err != nil {
		return EnvInfo{}, err
	}

	meta := getMetaInfo()

	runtime := getRuntimeInfo(configFile, config)

	host, err := getHostInfo()
	if err != nil {
		return EnvInfo{}, err
	}

	proxy := getProxyInfo(config)

	netmon := getNetmonInfo(config)

	shim, err := getShimInfo(config)
	if err != nil {
		return EnvInfo{}, err
	}

	agent, err := getAgentInfo(config)
	if err != nil {
		return EnvInfo{}, err
	}

	hypervisor := getHypervisorInfo(config)

	image := ImageInfo{
		Path: config.HypervisorConfig.ImagePath,
	}

	kernel := KernelInfo{
		Path:       config.HypervisorConfig.KernelPath,
		Parameters: strings.Join(vc.SerializeParams(config.HypervisorConfig.KernelParams, "="), " "),
	}

	initrd := InitrdInfo{
		Path: config.HypervisorConfig.InitrdPath,
	}

	env = EnvInfo{
		Meta:       meta,
		Runtime:    runtime,
		Hypervisor: hypervisor,
		Image:      image,
		Kernel:     kernel,
		Initrd:     initrd,
		Proxy:      proxy,
		Shim:       shim,
		Agent:      agent,
		Host:       host,
		Netmon:     netmon,
	}

	return env, nil
}

func handleSettings(file *os.File, c *cli.Context) error {
	if file == nil {
		return errors.New("Invalid output file specified")
	}

	configFile, ok := c.App.Metadata["configFile"].(string)
	if !ok {
		return errors.New("cannot determine config file")
	}

	runtimeConfig, ok := c.App.Metadata["runtimeConfig"].(oci.RuntimeConfig)
	if !ok {
		return errors.New("cannot determine runtime config")
	}

	env, err := getEnvInfo(configFile, runtimeConfig)
	if err != nil {
		return err
	}

	if c.Bool("json") {
		return writeJSONSettings(env, file)
	}

	return writeTOMLSettings(env, file)
}

func writeTOMLSettings(env EnvInfo, file *os.File) error {
	encoder := toml.NewEncoder(file)

	err := encoder.Encode(env)
	if err != nil {
		return err
	}

	return nil
}

func writeJSONSettings(env EnvInfo, file *os.File) error {
	encoder := json.NewEncoder(file)

	// Make it more human readable
	encoder.SetIndent("", "  ")

	err := encoder.Encode(env)
	if err != nil {
		return err
	}

	return nil
}

var kataEnvCLICommand = cli.Command{
	Name:  envCmd,
	Usage: "display settings. Default to TOML",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "json",
			Usage: "Format output as JSON",
		},
	},
	Action: func(context *cli.Context) error {
		ctx, err := cliContextToContext(context)
		if err != nil {
			return err
		}

		span, _ := katautils.Trace(ctx, "kata-env")
		defer span.Finish()

		return handleSettings(defaultOutputFile, context)
	},
}
