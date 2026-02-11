package hypeman

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var instanceCmd = &cobra.Command{
	Use:     "instance",
	Aliases: []string{"instances"},
	Short:   "Manage Hypeman instances",
}

var instanceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create and start an instance",
	RunE:  runInstanceCreate,
}

var instanceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List instances",
	RunE:  runInstanceList,
}

var instanceGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get instance details",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceGet,
}

var instanceDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Stop and delete an instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceDelete,
}

var instanceStartCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start a stopped instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceStart,
}

var instanceStopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop an instance (graceful shutdown)",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceStop,
}

var instanceRestoreCmd = &cobra.Command{
	Use:   "restore <id>",
	Short: "Restore an instance from standby",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceRestore,
}

var instanceStandbyCmd = &cobra.Command{
	Use:   "standby <id>",
	Short: "Put an instance in standby (pause, snapshot, delete VMM)",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceStandby,
}

var instanceLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Stream instance logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceLogs,
}

var instanceStatCmd = &cobra.Command{
	Use:   "stat <id>",
	Short: "Get file information from the guest filesystem",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceStat,
}

var instanceVolumeAttachCmd = &cobra.Command{
	Use:   "attach <volume-id>",
	Short: "Attach a volume to an instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceVolumeAttach,
}

var instanceVolumeDetachCmd = &cobra.Command{
	Use:   "detach <volume-id>",
	Short: "Detach a volume from an instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceVolumeDetach,
}

var instanceVolumeCmd = &cobra.Command{
	Use:   "volume",
	Short: "Manage instance volume attachments",
}

func init() {
	instanceCmd.AddCommand(instanceCreateCmd)
	instanceCmd.AddCommand(instanceListCmd)
	instanceCmd.AddCommand(instanceGetCmd)
	instanceCmd.AddCommand(instanceDeleteCmd)
	instanceCmd.AddCommand(instanceStartCmd)
	instanceCmd.AddCommand(instanceStopCmd)
	instanceCmd.AddCommand(instanceRestoreCmd)
	instanceCmd.AddCommand(instanceStandbyCmd)
	instanceCmd.AddCommand(instanceLogsCmd)
	instanceCmd.AddCommand(instanceStatCmd)
	instanceCmd.AddCommand(instanceVolumeCmd)
	instanceVolumeCmd.AddCommand(instanceVolumeAttachCmd)
	instanceVolumeCmd.AddCommand(instanceVolumeDetachCmd)

	// instance create flags
	instanceCreateCmd.Flags().String("image", "", "OCI image reference (required)")
	instanceCreateCmd.Flags().String("name", "", "Human-readable name (required)")
	_ = instanceCreateCmd.MarkFlagRequired("image")
	_ = instanceCreateCmd.MarkFlagRequired("name")
	instanceCreateCmd.Flags().String("size", "", "Base memory size (e.g., '1GB', '512MB')")
	instanceCreateCmd.Flags().String("hotplug-size", "", "Additional memory for hotplug (e.g., '3GB')")
	instanceCreateCmd.Flags().String("overlay-size", "", "Writable overlay disk size (e.g., '10GB')")
	instanceCreateCmd.Flags().String("disk-io-bps", "", "Disk I/O rate limit (e.g., '100MB/s')")
	instanceCreateCmd.Flags().Int64("vcpus", 0, "Number of virtual CPUs")
	instanceCreateCmd.Flags().Bool("skip-guest-agent", false, "Skip guest-agent installation during boot")
	instanceCreateCmd.Flags().Bool("skip-kernel-headers", false, "Skip kernel headers installation during boot")
	instanceCreateCmd.Flags().StringSlice("devices", nil, "Device IDs or names to attach for GPU/PCI passthrough")
	instanceCreateCmd.Flags().StringArrayP("env", "e", nil, "Environment variables (KEY=value, repeatable)")
	instanceCreateCmd.Flags().String("gpu-profile", "", "vGPU profile name (e.g., 'L40S-1Q')")
	instanceCreateCmd.Flags().String("hypervisor", "", "Hypervisor to use (cloud-hypervisor or qemu)")
	instanceCreateCmd.Flags().Bool("network-enabled", true, "Attach instance to the default network")
	instanceCreateCmd.Flags().String("bandwidth-download", "", "Download bandwidth limit (e.g., '1Gbps')")
	instanceCreateCmd.Flags().String("bandwidth-upload", "", "Upload bandwidth limit (e.g., '1Gbps')")
	instanceCreateCmd.Flags().StringArray("volume", nil, "Volume mounts (volume-id:mount-path[:ro], repeatable)")
	instanceCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	// instance list flags
	instanceListCmd.Flags().StringP("output", "o", "", "Output format: json")

	// instance get flags
	instanceGetCmd.Flags().StringP("output", "o", "", "Output format: json")

	// instance logs flags
	instanceLogsCmd.Flags().BoolP("follow", "f", false, "Continue streaming new lines")
	instanceLogsCmd.Flags().Int64("tail", 0, "Number of lines to return from end")
	instanceLogsCmd.Flags().String("source", "", "Log source: app, vmm, or hypeman")

	// instance stat flags
	instanceStatCmd.Flags().String("path", "", "Path to stat in the guest filesystem (required)")
	_ = instanceStatCmd.MarkFlagRequired("path")
	instanceStatCmd.Flags().Bool("follow-links", false, "Follow symbolic links")
	instanceStatCmd.Flags().StringP("output", "o", "", "Output format: json")

	// instance volume attach flags
	instanceVolumeAttachCmd.Flags().String("instance-id", "", "Instance ID (required)")
	_ = instanceVolumeAttachCmd.MarkFlagRequired("instance-id")
	instanceVolumeAttachCmd.Flags().String("mount-path", "", "Path where volume should be mounted (required)")
	_ = instanceVolumeAttachCmd.MarkFlagRequired("mount-path")
	instanceVolumeAttachCmd.Flags().Bool("readonly", false, "Mount as read-only")
	instanceVolumeAttachCmd.Flags().StringP("output", "o", "", "Output format: json")

	// instance volume detach flags
	instanceVolumeDetachCmd.Flags().String("instance-id", "", "Instance ID (required)")
	_ = instanceVolumeDetachCmd.MarkFlagRequired("instance-id")
	instanceVolumeDetachCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runInstanceCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	image, _ := cmd.Flags().GetString("image")
	name, _ := cmd.Flags().GetString("name")

	params := hypemansdk.InstanceNewParams{
		Image: image,
		Name:  name,
	}

	if v, _ := cmd.Flags().GetString("size"); v != "" {
		params.Size = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("hotplug-size"); v != "" {
		params.HotplugSize = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("overlay-size"); v != "" {
		params.OverlaySize = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("disk-io-bps"); v != "" {
		params.DiskIoBps = param.NewOpt(v)
	}
	if cmd.Flags().Changed("vcpus") {
		v, _ := cmd.Flags().GetInt64("vcpus")
		params.Vcpus = param.NewOpt(v)
	}
	if cmd.Flags().Changed("skip-guest-agent") {
		v, _ := cmd.Flags().GetBool("skip-guest-agent")
		params.SkipGuestAgent = param.NewOpt(v)
	}
	if cmd.Flags().Changed("skip-kernel-headers") {
		v, _ := cmd.Flags().GetBool("skip-kernel-headers")
		params.SkipKernelHeaders = param.NewOpt(v)
	}
	if devices, _ := cmd.Flags().GetStringSlice("devices"); len(devices) > 0 {
		params.Devices = devices
	}
	if envPairs, _ := cmd.Flags().GetStringArray("env"); len(envPairs) > 0 {
		envMap := make(map[string]string)
		for _, kv := range envPairs {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid env format: %s (expected KEY=value)", kv)
			}
			envMap[parts[0]] = parts[1]
		}
		params.Env = envMap
	}
	if v, _ := cmd.Flags().GetString("gpu-profile"); v != "" {
		params.GPU = hypemansdk.InstanceNewParamsGPU{
			Profile: param.NewOpt(v),
		}
	}
	if v, _ := cmd.Flags().GetString("hypervisor"); v != "" {
		params.Hypervisor = hypemansdk.InstanceNewParamsHypervisor(v)
	}
	if cmd.Flags().Changed("network-enabled") {
		v, _ := cmd.Flags().GetBool("network-enabled")
		params.Network = hypemansdk.InstanceNewParamsNetwork{
			Enabled: param.NewOpt(v),
		}
	}
	if v, _ := cmd.Flags().GetString("bandwidth-download"); v != "" {
		params.Network.BandwidthDownload = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("bandwidth-upload"); v != "" {
		params.Network.BandwidthUpload = param.NewOpt(v)
	}
	if volumeSpecs, _ := cmd.Flags().GetStringArray("volume"); len(volumeSpecs) > 0 {
		var mounts []hypemansdk.VolumeMountParam
		for _, spec := range volumeSpecs {
			parts := strings.SplitN(spec, ":", 3)
			if len(parts) < 2 {
				return fmt.Errorf("invalid volume format: %s (expected volume-id:mount-path[:ro])", spec)
			}
			mount := hypemansdk.VolumeMountParam{
				VolumeID:  parts[0],
				MountPath: parts[1],
			}
			if len(parts) == 3 && parts[2] == "ro" {
				mount.Readonly = param.NewOpt(true)
			}
			mounts = append(mounts, mount)
		}
		params.Volumes = mounts
	}

	instance, err := client.Instances.New(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(instance)
	}

	pterm.Success.Printf("Created instance %s (%s)\n", instance.Name, instance.ID)
	printInstanceDetail(instance)
	return nil
}

func runInstanceList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	instances, err := client.Instances.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	if output == "json" {
		if instances == nil || len(*instances) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*instances)
	}

	if instances == nil || len(*instances) == 0 {
		pterm.Info.Println("No instances found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Name", "Image", "State", "vCPUs", "Size", "Created At"}}
	for _, inst := range *instances {
		tableData = append(tableData, []string{
			inst.ID,
			inst.Name,
			inst.Image,
			string(inst.State),
			fmt.Sprintf("%d", inst.Vcpus),
			util.OrDash(inst.Size),
			util.FormatLocal(inst.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runInstanceGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	instance, err := client.Instances.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(instance)
	}

	printInstanceDetail(instance)
	return nil
}

func runInstanceDelete(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Instances.Delete(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	pterm.Success.Printf("Deleted instance %s\n", args[0])
	return nil
}

func runInstanceStart(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	instance, err := client.Instances.Start(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	pterm.Success.Printf("Started instance %s (state: %s)\n", instance.Name, instance.State)
	return nil
}

func runInstanceStop(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	instance, err := client.Instances.Stop(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	pterm.Success.Printf("Stopped instance %s (state: %s)\n", instance.Name, instance.State)
	return nil
}

func runInstanceRestore(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	instance, err := client.Instances.Restore(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to restore instance: %w", err)
	}

	pterm.Success.Printf("Restored instance %s (state: %s)\n", instance.Name, instance.State)
	return nil
}

func runInstanceStandby(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	instance, err := client.Instances.Standby(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to put instance in standby: %w", err)
	}

	pterm.Success.Printf("Instance %s is now in standby (state: %s)\n", instance.Name, instance.State)
	return nil
}

func runInstanceLogs(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt64("tail")
	source, _ := cmd.Flags().GetString("source")

	params := hypemansdk.InstanceLogsParams{
		Follow: param.NewOpt(follow),
	}
	if tail > 0 {
		params.Tail = param.NewOpt(tail)
	}
	if source != "" {
		params.Source = hypemansdk.InstanceLogsParamsSource(source)
	}

	stream := client.Instances.LogsStreaming(cmd.Context(), args[0], params)
	for stream.Next() {
		fmt.Println(stream.Current())
	}
	if stream.Err() != nil {
		return fmt.Errorf("log stream error: %w", stream.Err())
	}
	return nil
}

func runInstanceStat(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	path, _ := cmd.Flags().GetString("path")

	params := hypemansdk.InstanceStatParams{
		Path: path,
	}
	if cmd.Flags().Changed("follow-links") {
		v, _ := cmd.Flags().GetBool("follow-links")
		params.FollowLinks = param.NewOpt(v)
	}

	info, err := client.Instances.Stat(cmd.Context(), args[0], params)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(info)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Exists", fmt.Sprintf("%t", info.Exists)},
		{"Is File", fmt.Sprintf("%t", info.IsFile)},
		{"Is Dir", fmt.Sprintf("%t", info.IsDir)},
		{"Is Symlink", fmt.Sprintf("%t", info.IsSymlink)},
		{"Size", fmt.Sprintf("%d", info.Size)},
		{"Mode", fmt.Sprintf("%o", info.Mode)},
	}
	if info.LinkTarget != "" {
		tableData = append(tableData, []string{"Link Target", info.LinkTarget})
	}
	if info.Error != "" {
		tableData = append(tableData, []string{"Error", info.Error})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runInstanceVolumeAttach(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	instanceID, _ := cmd.Flags().GetString("instance-id")
	mountPath, _ := cmd.Flags().GetString("mount-path")

	params := hypemansdk.InstanceVolumeAttachParams{
		ID:        instanceID,
		MountPath: mountPath,
	}
	if cmd.Flags().Changed("readonly") {
		v, _ := cmd.Flags().GetBool("readonly")
		params.Readonly = param.NewOpt(v)
	}

	instance, err := client.Instances.Volumes.Attach(cmd.Context(), args[0], params)
	if err != nil {
		return fmt.Errorf("failed to attach volume: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(instance)
	}

	pterm.Success.Printf("Attached volume %s to instance %s at %s\n", args[0], instanceID, mountPath)
	return nil
}

func runInstanceVolumeDetach(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	instanceID, _ := cmd.Flags().GetString("instance-id")

	params := hypemansdk.InstanceVolumeDetachParams{
		ID: instanceID,
	}

	instance, err := client.Instances.Volumes.Detach(cmd.Context(), args[0], params)
	if err != nil {
		return fmt.Errorf("failed to detach volume: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(instance)
	}

	pterm.Success.Printf("Detached volume %s from instance %s\n", args[0], instanceID)
	return nil
}

func printInstanceDetail(inst *hypemansdk.Instance) {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", inst.ID},
		{"Name", inst.Name},
		{"Image", inst.Image},
		{"State", string(inst.State)},
		{"vCPUs", fmt.Sprintf("%d", inst.Vcpus)},
		{"Size", util.OrDash(inst.Size)},
		{"Overlay Size", util.OrDash(inst.OverlaySize)},
		{"Hotplug Size", util.OrDash(inst.HotplugSize)},
		{"Disk I/O BPS", util.OrDash(inst.DiskIoBps)},
		{"Hypervisor", string(inst.Hypervisor)},
		{"Has Snapshot", fmt.Sprintf("%t", inst.HasSnapshot)},
		{"Created At", util.FormatLocal(inst.CreatedAt)},
	}
	if inst.Network.Enabled {
		tableData = append(tableData, []string{"Network IP", util.OrDash(inst.Network.IP)})
	}
	if len(inst.Volumes) > 0 {
		var volStrs []string
		for _, v := range inst.Volumes {
			volStrs = append(volStrs, fmt.Sprintf("%s@%s", v.VolumeID, v.MountPath))
		}
		tableData = append(tableData, []string{"Volumes", strings.Join(volStrs, ", ")})
	}
	if len(inst.Env) > 0 {
		envBytes, _ := json.Marshal(inst.Env)
		tableData = append(tableData, []string{"Env", string(envBytes)})
	}
	table.PrintTableNoPad(tableData, true)
}
