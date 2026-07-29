package proxies

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func (p ProxyCmd) Create(ctx context.Context, in ProxyCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	// Validate proxy type
	var proxyType kernel.ProxyNewParamsType
	switch in.Type {
	case "datacenter":
		proxyType = kernel.ProxyNewParamsTypeDatacenter
	case "isp":
		proxyType = kernel.ProxyNewParamsTypeIsp
	case "residential":
		proxyType = kernel.ProxyNewParamsTypeResidential
	case "mobile":
		proxyType = kernel.ProxyNewParamsTypeMobile
	case "custom":
		proxyType = kernel.ProxyNewParamsTypeCustom
	default:
		return fmt.Errorf("invalid proxy type: %s", in.Type)
	}

	params := kernel.ProxyNewParams{
		Type: proxyType,
	}

	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}
	if len(in.BypassHosts) > 0 {
		params.BypassHosts = normalizeBypassHosts(in.BypassHosts)
	}

	// Build config based on type
	switch proxyType {
	case kernel.ProxyNewParamsTypeDatacenter:
		config := kernel.ProxyNewParamsConfigDatacenter{}
		if in.Country != "" {
			config.Country = kernel.Opt(in.Country)
		}
		params.Config = kernel.ProxyNewParamsConfigUnion{
			OfDatacenter: &config,
		}

	case kernel.ProxyNewParamsTypeIsp:
		config := kernel.ProxyNewParamsConfigIsp{}
		if in.Country != "" {
			config.Country = kernel.Opt(in.Country)
		}
		params.Config = kernel.ProxyNewParamsConfigUnion{
			OfIsp: &config,
		}

	case kernel.ProxyNewParamsTypeResidential:
		config := kernel.ProxyNewParamsConfigResidential{}

		// Validate that if city is provided, country must also be provided
		if in.City != "" && in.Country == "" {
			return fmt.Errorf("--country is required when --city is specified")
		}

		if in.Country != "" {
			config.Country = kernel.Opt(in.Country)
		}
		if in.City != "" {
			config.City = kernel.Opt(in.City)
		}
		if in.State != "" {
			config.State = kernel.Opt(in.State)
		}
		if in.Zip != "" {
			config.Zip = kernel.Opt(in.Zip)
		}
		if in.ASN != "" {
			config.Asn = kernel.Opt(in.ASN)
		}
		if in.OS != "" {
			// Validate OS value
			switch in.OS {
			case "windows", "macos", "android":
				config.Os = in.OS
			default:
				return fmt.Errorf("invalid OS value: %s (must be windows, macos, or android)", in.OS)
			}
		}
		params.Config = kernel.ProxyNewParamsConfigUnion{
			OfResidential: &config,
		}

	case kernel.ProxyNewParamsTypeMobile:
		config := kernel.ProxyNewParamsConfigMobile{}

		// Validate that if city is provided, country must also be provided
		if in.City != "" && in.Country == "" {
			return fmt.Errorf("--country is required when --city is specified")
		}
		if in.Zip != "" || in.ASN != "" {
			pterm.Warning.Println("--zip and --asn are not supported for mobile proxies and will be ignored")
		}

		if in.Country != "" {
			config.Country = kernel.Opt(in.Country)
		}
		if in.City != "" {
			config.City = kernel.Opt(in.City)
		}
		if in.State != "" {
			config.State = kernel.Opt(in.State)
		}
		params.Config = kernel.ProxyNewParamsConfigUnion{
			OfMobile: &config,
		}

	case kernel.ProxyNewParamsTypeCustom:
		if in.Host == "" {
			return fmt.Errorf("--host is required for custom proxy type")
		}
		if in.Port == 0 {
			return fmt.Errorf("--port is required for custom proxy type")
		}

		caBundle, err := resolveCaBundle(in.CaBundle, in.CaBundleFile)
		if err != nil {
			return err
		}

		config := kernel.ProxyNewParamsConfigCustom{
			Host: in.Host,
			Port: int64(in.Port),
		}
		if in.Username != "" {
			config.Username = kernel.Opt(in.Username)
		}
		if in.Password != "" {
			config.Password = kernel.Opt(in.Password)
		}
		if caBundle != "" {
			config.CaBundle = kernel.Opt(caBundle)
		}
		params.Config = kernel.ProxyNewParamsConfigUnion{
			OfCustom: &config,
		}
	}

	if proxyType != kernel.ProxyNewParamsTypeCustom && (in.CaBundle != "" || in.CaBundleFile != "") {
		pterm.Warning.Println("--ca-bundle is only supported for custom proxies and will be ignored")
	}

	// Set protocol (defaults to https if not specified)
	if in.Protocol != "" {
		// Validate and convert protocol
		switch in.Protocol {
		case "http":
			params.Protocol = kernel.ProxyNewParamsProtocolHTTP
		case "https":
			params.Protocol = kernel.ProxyNewParamsProtocolHTTPS
		default:
			return fmt.Errorf("invalid protocol: %s (must be http or https)", in.Protocol)
		}
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating %s proxy...\n", proxyType)
	}

	proxy, err := p.proxies.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(proxy)
	}

	pterm.Success.Printf("Successfully created proxy\n")

	// Display created proxy details
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", proxy.ID})

	name := proxy.Name
	if name == "" {
		name = "-"
	}
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Type", string(proxy.Type)})
	rows = append(rows, []string{"Bypass Hosts", formatBypassHosts(proxy.BypassHosts)})

	// Display protocol (default to https if not set)
	protocol := string(proxy.Protocol)
	if protocol == "" {
		protocol = "https"
	}
	rows = append(rows, []string{"Protocol", protocol})

	// The CA bundle is write-only, so confirm the API stored it.
	if proxy.Config.HasCaBundle {
		rows = append(rows, []string{"Has CA Bundle", "Yes"})
	}

	table.PrintTableNoPad(rows, true)
	return nil
}

func runProxiesCreate(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)

	// Get all flag values
	proxyType, _ := cmd.Flags().GetString("type")
	name, _ := cmd.Flags().GetString("name")
	protocol, _ := cmd.Flags().GetString("protocol")
	country, _ := cmd.Flags().GetString("country")
	city, _ := cmd.Flags().GetString("city")
	state, _ := cmd.Flags().GetString("state")
	zip, _ := cmd.Flags().GetString("zip")
	asn, _ := cmd.Flags().GetString("asn")
	os, _ := cmd.Flags().GetString("os")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	caBundle, _ := cmd.Flags().GetString("ca-bundle")
	caBundleFile, _ := cmd.Flags().GetString("ca-bundle-file")
	bypassHosts, _ := cmd.Flags().GetStringSlice("bypass-host")

	output, _ := cmd.Flags().GetString("output")

	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Create(cmd.Context(), ProxyCreateInput{
		Name:         name,
		Type:         proxyType,
		Protocol:     protocol,
		BypassHosts:  bypassHosts,
		Country:      country,
		City:         city,
		State:        state,
		Zip:          zip,
		ASN:          asn,
		OS:           os,
		Host:         host,
		Port:         port,
		Username:     username,
		Password:     password,
		CaBundle:     caBundle,
		CaBundleFile: caBundleFile,
		Output:       output,
	})
}

// resolveCaBundle resolves the --ca-bundle / --ca-bundle-file inputs into a
// PEM-encoded CA certificate bundle. The two inputs are mutually exclusive
// (enforced by cobra); a file path of "-" reads stdin. It returns an empty
// string when neither input is set.
func resolveCaBundle(inline, file string) (string, error) {
	data := inline
	if file != "" {
		var b []byte
		var err error
		if file == "-" {
			b, err = io.ReadAll(os.Stdin)
		} else {
			b, err = os.ReadFile(file)
		}
		if err != nil {
			return "", fmt.Errorf("failed to read CA bundle file: %w", err)
		}
		data = string(b)
	}

	data = strings.TrimSpace(data)
	if data == "" {
		return "", nil
	}
	if !strings.Contains(data, "-----BEGIN ") {
		return "", fmt.Errorf("invalid CA bundle: expected PEM-encoded certificate data (a '-----BEGIN CERTIFICATE-----' block)")
	}

	return data, nil
}

func normalizeBypassHosts(hosts []string) []string {
	normalized := make([]string, 0, len(hosts))
	for _, host := range hosts {
		trimmed := strings.TrimSpace(host)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}

	return normalized
}
