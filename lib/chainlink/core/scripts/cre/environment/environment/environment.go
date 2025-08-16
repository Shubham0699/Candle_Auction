package environment

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cldlogger "github.com/smartcontractkit/chainlink/deployment/logger"

	"github.com/smartcontractkit/chainlink/core/scripts/cre/environment/tracking"
	libc "github.com/smartcontractkit/chainlink/system-tests/lib/conversions"
	crecapabilities "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities"
	computecap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/compute"
	consensuscap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/consensus"
	croncap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/cron"
	logeventtriggercap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/logevent"
	readcontractcap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/readcontract"
	webapicap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/webapi"
	writeevmcap "github.com/smartcontractkit/chainlink/system-tests/lib/cre/capabilities/writeevm"
	gatewayconfig "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/config/gateway"
	crecompute "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/compute"
	creconsensus "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/consensus"
	crecron "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/cron"
	cregateway "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/gateway"
	crelogevent "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/logevent"
	crereadcontract "github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/readcontract"
	"github.com/smartcontractkit/chainlink/system-tests/lib/cre/don/jobs/webapi"
	creenv "github.com/smartcontractkit/chainlink/system-tests/lib/cre/environment"
	cretypes "github.com/smartcontractkit/chainlink/system-tests/lib/cre/types"
	"github.com/smartcontractkit/chainlink/system-tests/lib/crecli"
	libformat "github.com/smartcontractkit/chainlink/system-tests/lib/format"
	libtypes "github.com/smartcontractkit/chainlink/system-tests/lib/types"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
	chipingressset "github.com/smartcontractkit/chainlink-testing-framework/framework/components/dockercompose/chip_ingress_set"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/jd"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/s3provider"
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/ptr"
)

const manualCtfCleanupMsg = `unexpected startup error. this may have stranded resources. please manually remove containers with 'ctf' label and delete their volumes`
const manualBeholderCleanupMsg = `unexpected startup error. this may have stranded resources. please manually remove the 'chip-ingress' stack`

func init() {
	EnvironmentCmd.AddCommand(startCmd())
	EnvironmentCmd.AddCommand(stopCmd)
	EnvironmentCmd.AddCommand(workflowCmds())
	EnvironmentCmd.AddCommand(beholderCmds())
}

func waitToCleanUp(d time.Duration) {
	fmt.Printf("Waiting %s before cleanup\n", d)
	time.Sleep(d)
}

var EnvironmentCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment commands",
	Long:  `Commands to manage the environment`,
}

const (
	TopologySimplified = "simplified"
	TopologyFull       = "full"

	WorkflowTriggerWebTrigger = "web-trigger"
	WorkflowTriggerCron       = "cron"
)

type Config struct {
	Blockchains       []*cretypes.WrappedBlockchainInput `toml:"blockchains" validate:"required"`
	NodeSets          []*ns.Input                        `toml:"nodesets" validate:"required"`
	JD                *jd.Input                          `toml:"jd" validate:"required"`
	Infra             *libtypes.InfraInput               `toml:"infra" validate:"required"`
	ExtraCapabilities ExtraCapabilitiesConfig            `toml:"extra_capabilities"`
	S3ProviderInput   *s3provider.Input                  `toml:"s3provider"`
}

func (c Config) Validate() error {
	if c.JD.CSAEncryptionKey == "" {
		return errors.New("jd.csa_encryption_key must be provided")
	}
	return nil
}

type ExtraCapabilitiesConfig struct {
	CronCapabilityBinaryPath  string `toml:"cron_capability_binary_path"`
	LogEventTriggerBinaryPath string `toml:"log_event_trigger_binary_path"`
	ReadContractBinaryPath    string `toml:"read_contract_capability_binary_path"`
}

// DX tracking
var (
	dxTracker             tracking.Tracker
	provisioningStartTime time.Time
)

var StartCmdPreRunFunc = func(cmd *cobra.Command, args []string) {
	provisioningStartTime = time.Now()

	// ensure non-nil dxTracker by default
	var trackerErr error
	dxTracker, trackerErr = tracking.NewDxTracker()
	if trackerErr != nil {
		fmt.Fprintf(os.Stderr, "failed to create DX tracker: %s\n", trackerErr)
		dxTracker = &tracking.NoOpTracker{}
	}

	// remove all containers before starting the environment, just in case
	_ = framework.RemoveTestContainers()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal: %s\n", sig)

		removeErr := framework.RemoveTestContainers()
		if removeErr != nil {
			fmt.Fprint(os.Stderr, removeErr, manualCtfCleanupMsg)
		}

		os.Exit(1)
	}()
}

var StartCmdRecoverHandlerFunc = func(p interface{}, cleanupWait time.Duration) {
	if p != nil {
		fmt.Println("Panicked when starting environment")

		var errText string
		if err, ok := p.(error); ok {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			fmt.Fprintf(os.Stderr, "Stack trace: %s\n", string(debug.Stack()))

			errText = strings.SplitN(err.Error(), "\n", 1)[0]
		} else {
			fmt.Fprintf(os.Stderr, "panic: %v\n", p)
			fmt.Fprintf(os.Stderr, "Stack trace: %s\n", string(debug.Stack()))

			errText = strings.SplitN(fmt.Sprintf("%v", p), "\n", 1)[0]
		}

		tracingErr := dxTracker.Track("startup.result", map[string]any{
			"success":  false,
			"error":    errText,
			"panicked": true,
		})

		if tracingErr != nil {
			fmt.Fprintf(os.Stderr, "failed to track startup: %s\n", tracingErr)
		}

		waitToCleanUp(cleanupWait)

		removeErr := framework.RemoveTestContainers()
		if removeErr != nil {
			fmt.Fprint(os.Stderr, errors.Wrap(removeErr, manualCtfCleanupMsg).Error())
		}
	}
}

var StartCmdGenerateSettingsFile = func(homeChainOut *cretypes.WrappedBlockchainOutput, output *creenv.SetupOutput) error {
	rpcs := map[uint64]string{}
	for _, bcOut := range output.BlockchainOutput {
		rpcs[bcOut.ChainSelector] = bcOut.BlockchainOutput.Nodes[0].ExternalHTTPUrl
	}

	creCLISettingsFile, settingsErr := crecli.PrepareCRECLISettingsFile(
		crecli.CRECLIProfile,
		homeChainOut.SethClient.MustGetRootKeyAddress(),
		output.CldEnvironment.ExistingAddresses, //nolint:staticcheck,nolintlint // SA1019: deprecated but we don't want to migrate now
		output.DonTopology.WorkflowDonID,
		homeChainOut.ChainSelector,
		rpcs,
		output.S3ProviderOutput,
	)

	if settingsErr != nil {
		return settingsErr
	}

	// Copy the file to current directory as cre.yaml
	currentDir, cErr := os.Getwd()
	if cErr != nil {
		return cErr
	}

	targetPath := filepath.Join(currentDir, "cre.yaml")
	input, err := os.ReadFile(creCLISettingsFile.Name())
	if err != nil {
		return err
	}
	err = os.WriteFile(targetPath, input, 0600)
	if err != nil {
		return err
	}

	fmt.Printf("CRE CLI settings file created: %s\n\n", targetPath)

	return nil
}

func startCmd() *cobra.Command {
	var (
		topology                 string
		extraAllowedGatewayPorts []int
		withExampleFlag          bool
		exampleWorkflowTrigger   string
		exampleWorkflowTimeout   time.Duration
		withPluginsDockerImage   string
		doSetup                  bool
		cleanupWait              time.Duration
		withBeholder             bool
		protoConfigs             []string
	)

	cmd := &cobra.Command{
		Use:              "start",
		Short:            "Start the environment",
		Long:             `Start the local CRE environment with all supported capabilities`,
		PersistentPreRun: StartCmdPreRunFunc,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() {
				p := recover()
				StartCmdRecoverHandlerFunc(p, cleanupWait)
			}()

			if doSetup {
				setupErr := RunSetup(cmd.Context(), SetupConfig{})
				if setupErr != nil {
					return errors.Wrap(setupErr, "failed to run setup")
				}
			}

			if topology != TopologySimplified && topology != TopologyFull {
				return fmt.Errorf("invalid topology: %s. Valid topologies are: %s, %s", topology, TopologySimplified, TopologyFull)
			}

			PrintCRELogo()

			if err := defaultCtfConfigs(topology); err != nil {
				return errors.Wrap(err, "failed to set default CTF configs")
			}

			if os.Getenv("PRIVATE_KEY") == "" {
				setErr := os.Setenv("PRIVATE_KEY", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
				if setErr != nil {
					return fmt.Errorf("failed to set PRIVATE_KEY environment variable: %w", setErr)
				}
				fmt.Printf("Set PRIVATE_KEY environment variable to default value: %s\n", os.Getenv("PRIVATE_KEY"))
			}

			// set TESTCONTAINERS_RYUK_DISABLED to true to disable Ryuk, so that Ryuk doesn't destroy the containers, when the command ends
			setErr := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
			if setErr != nil {
				return fmt.Errorf("failed to set TESTCONTAINERS_RYUK_DISABLED environment variable: %w", setErr)
			}

			cmdContext := cmd.Context()
			// Load and validate test configuration
			in, err := framework.Load[Config](nil)
			if err != nil {
				return errors.Wrap(err, "failed to load test configuration")
			}
			if err := in.Validate(); err != nil {
				return errors.Wrap(err, "failed to validate test configuration")
			}

			output, startErr := StartCLIEnvironment(cmdContext, in, topology, exampleWorkflowTrigger, withPluginsDockerImage, withExampleFlag, extraAllowedGatewayPorts, nil, nil)
			if startErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", startErr)
				fmt.Fprintf(os.Stderr, "Stack trace: %s\n", string(debug.Stack()))

				dxErr := trackStartup(false, hasBuiltDockerImage(in, withPluginsDockerImage), in.Infra.InfraType, ptr.Ptr(strings.SplitN(startErr.Error(), "\n", 1)[0]), ptr.Ptr(false))
				if dxErr != nil {
					fmt.Fprintf(os.Stderr, "failed to track startup: %s\n", dxErr)
				}

				waitToCleanUp(cleanupWait)
				removeErr := framework.RemoveTestContainers()
				if removeErr != nil {
					return errors.Wrap(removeErr, manualCtfCleanupMsg)
				}

				return errors.Wrap(startErr, "failed to start environment")
			}

			homeChainOut := output.BlockchainOutput[0]

			sErr := StartCmdGenerateSettingsFile(homeChainOut, output)

			if sErr != nil {
				fmt.Fprintf(os.Stderr, "failed to create CRE CLI settings file: %s. You need to create it manually.", sErr)
			}

			dxErr := trackStartup(true, hasBuiltDockerImage(in, withPluginsDockerImage), output.InfraInput.InfraType, nil, nil)
			if dxErr != nil {
				fmt.Fprintf(os.Stderr, "failed to track startup: %s\n", dxErr)
			}

			if withBeholder {
				startBeholderErr := startBeholder(
					cmdContext,
					cleanupWait,
					protoConfigs,
					nil, // extra Docker network is not required, since chip-ingress will connect to default Framework network
				)
				if startBeholderErr != nil {
					if !strings.Contains(startBeholderErr.Error(), protoRegistrationErrMsg) {
						beholderRemoveErr := framework.RemoveTestStack(chipingressset.DEFAULT_STACK_NAME)
						if beholderRemoveErr != nil {
							fmt.Fprint(os.Stderr, errors.Wrap(beholderRemoveErr, manualBeholderCleanupMsg).Error())
						}
					}
					return errors.Wrap(startBeholderErr, "failed to start Beholder")
				}
			}

			if withExampleFlag {
				gatewayURL := fmt.Sprintf("%s://%s:%d%s", output.DonTopology.GatewayConnectorOutput.Incoming.Protocol, output.DonTopology.GatewayConnectorOutput.Incoming.Host, output.DonTopology.GatewayConnectorOutput.Incoming.ExternalPort, output.DonTopology.GatewayConnectorOutput.Incoming.Path)

				fmt.Print(libformat.PurpleText("\nRegistering and verifying example workflow\n\n"))
				deployErr := deployAndVerifyExampleWorkflow(cmdContext, homeChainOut.BlockchainOutput.Nodes[0].ExternalHTTPUrl, gatewayURL, homeChainOut.ChainID, exampleWorkflowTimeout, exampleWorkflowTrigger)
				if deployErr != nil {
					fmt.Printf("Failed to deploy and verify example workflow: %s\n", deployErr)
				}
			}
			fmt.Print(libformat.PurpleText("\nEnvironment setup completed successfully in %.2f seconds\n\n", time.Since(provisioningStartTime).Seconds()))
			fmt.Print("To terminate execute:`go run . env stop`\n\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&topology, "topology", "t", "simplified", "Topology to use for the environment (simplified or full)")
	cmd.Flags().DurationVarP(&cleanupWait, "wait-on-error-timeout", "w", 15*time.Second, "Wait on error timeout (e.g. 10s, 1m, 1h)")
	cmd.Flags().IntSliceVarP(&extraAllowedGatewayPorts, "extra-allowed-gateway-ports", "e", []int{}, "Extra allowed ports for outgoing connections from the Gateway DON (e.g. 8080,8081)")
	cmd.Flags().BoolVarP(&withExampleFlag, "with-example", "x", false, "Deploy and register example workflow")
	cmd.Flags().DurationVarP(&exampleWorkflowTimeout, "example-workflow-timeout", "u", 5*time.Minute, "Time to wait until example workflow succeeds")
	cmd.Flags().StringVarP(&withPluginsDockerImage, "with-plugins-docker-image", "p", "", "Docker image to use (must have all capabilities included)")
	cmd.Flags().StringVarP(&exampleWorkflowTrigger, "example-workflow-trigger", "y", "web-trigger", "Trigger for example workflow to deploy (web-trigger or cron)")
	cmd.Flags().BoolVarP(&withBeholder, "with-beholder", "b", false, "Deploy Beholder (Chip Ingress + Red Panda)")
	cmd.Flags().StringArrayVarP(&protoConfigs, "with-proto-configs", "c", []string{"./proto-configs/default.toml"}, "Protos configs to use (e.g. './proto-configs/config_one.toml,./proto-configs/config_two.toml')")
	cmd.Flags().BoolVarP(&doSetup, "auto-setup", "a", false, "Run setup before starting the environment")
	return cmd
}

func trackStartup(success, hasBuiltDockerImage bool, infraType string, errorMessage *string, panicked *bool) error {
	metadata := map[string]any{
		"success": success,
		"infra":   infraType,
	}

	if errorMessage != nil {
		metadata["error"] = *errorMessage
	}

	if panicked != nil {
		metadata["panicked"] = *panicked
	}

	dxStartupErr := dxTracker.Track("cre.local.startup.result", metadata)
	if dxStartupErr != nil {
		fmt.Fprintf(os.Stderr, "failed to track startup: %s\n", dxStartupErr)
	}

	if success {
		dxTimeErr := dxTracker.Track("cre.local.startup.time", map[string]any{
			"duration_seconds":       time.Since(provisioningStartTime).Seconds(),
			"has_built_docker_image": hasBuiltDockerImage,
		})

		if dxTimeErr != nil {
			fmt.Fprintf(os.Stderr, "failed to track startup time: %s\n", dxTimeErr)
		}
	}

	return nil
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stops the environment",
	Long:  `Stops the local CRE environment (if it's not running, it just fallsthrough)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		removeErr := removeAllContainers()
		if removeErr != nil {
			return errors.Wrap(removeErr, "failed to remove environment containers. Please remove them manually")
		}

		fmt.Println("Environment stopped successfully")
		return nil
	},
}

func removeAllContainers() error {
	ctfRemoveErr := framework.RemoveTestContainers()
	if ctfRemoveErr != nil {
		fmt.Fprint(os.Stderr, errors.Wrap(ctfRemoveErr, manualCtfCleanupMsg).Error())
	}

	beholderRemoveErr := framework.RemoveTestStack(chipingressset.DEFAULT_STACK_NAME)
	if beholderRemoveErr != nil {
		fmt.Fprint(os.Stderr, errors.Wrap(beholderRemoveErr, manualBeholderCleanupMsg).Error())
	}

	return nil
}

func StartCLIEnvironment(
	cmdContext context.Context,
	in *Config,
	topologyFlag string,
	workflowTrigger,
	withPluginsDockerImageFlag string,
	withExampleFlag bool,
	extraAllowedGatewayPorts []int,
	extraBinaries map[string]string,
	extraJobFactoryFns []cretypes.JobSpecFactoryFn,
) (*creenv.SetupOutput, error) {
	testLogger := framework.L

	// make sure that either cron is enabled or withPluginsDockerImageFlag is set, but only if workflowTrigger is cron
	if withExampleFlag && workflowTrigger == WorkflowTriggerCron && (in.ExtraCapabilities.CronCapabilityBinaryPath == "" && withPluginsDockerImageFlag == "") {
		return nil, fmt.Errorf("either cron binary path must be set in TOML config (%s) or you must use Docker image with all capabilities included and passed via withPluginsDockerImageFlag", os.Getenv("CTF_CONFIGS"))
	}

	capabilitiesBinaryPaths := map[cretypes.CapabilityFlag]string{}
	var capabilitiesAwareNodeSets []*cretypes.CapabilitiesAwareNodeSet

	if topologyFlag == TopologySimplified {
		if len(in.NodeSets) != 1 {
			return nil, fmt.Errorf("expected 1 nodeset, got %d", len(in.NodeSets))
		}
		// add support for more binaries if needed
		workflowDONCapabilities := []string{cretypes.OCR3Capability, cretypes.CustomComputeCapability, cretypes.WriteEVMCapability, cretypes.WebAPITriggerCapability, cretypes.WebAPITargetCapability}
		if in.ExtraCapabilities.CronCapabilityBinaryPath != "" || withPluginsDockerImageFlag != "" {
			workflowDONCapabilities = append(workflowDONCapabilities, cretypes.CronCapability)
			capabilitiesBinaryPaths[cretypes.CronCapability] = in.ExtraCapabilities.CronCapabilityBinaryPath
		}

		if in.ExtraCapabilities.LogEventTriggerBinaryPath != "" || withPluginsDockerImageFlag != "" {
			workflowDONCapabilities = append(workflowDONCapabilities, cretypes.LogTriggerCapability)
			capabilitiesBinaryPaths[cretypes.LogTriggerCapability] = in.ExtraCapabilities.LogEventTriggerBinaryPath
		}

		if in.ExtraCapabilities.ReadContractBinaryPath != "" || withPluginsDockerImageFlag != "" {
			workflowDONCapabilities = append(workflowDONCapabilities, cretypes.ReadContractCapability)
			capabilitiesBinaryPaths[cretypes.ReadContractCapability] = in.ExtraCapabilities.ReadContractBinaryPath
		}

		for capabilityName, binaryPath := range extraBinaries {
			if binaryPath != "" || withPluginsDockerImageFlag != "" {
				workflowDONCapabilities = append(workflowDONCapabilities, capabilityName)
				capabilitiesBinaryPaths[capabilityName] = binaryPath
			}
		}

		capabilitiesAwareNodeSets = []*cretypes.CapabilitiesAwareNodeSet{
			{
				Input:              in.NodeSets[0],
				Capabilities:       workflowDONCapabilities,
				DONTypes:           []string{cretypes.WorkflowDON, cretypes.GatewayDON},
				BootstrapNodeIndex: 0,
				GatewayNodeIndex:   0,
			},
		}
	} else {
		if len(in.NodeSets) != 3 {
			return nil, fmt.Errorf("expected 3 nodesets, got %d", len(in.NodeSets))
		}

		// add support for more binaries if needed
		workflowDONCapabilities := []string{cretypes.OCR3Capability, cretypes.CustomComputeCapability, cretypes.WebAPITriggerCapability}
		if in.ExtraCapabilities.CronCapabilityBinaryPath != "" || withPluginsDockerImageFlag != "" {
			workflowDONCapabilities = append(workflowDONCapabilities, cretypes.CronCapability)
			capabilitiesBinaryPaths[cretypes.CronCapability] = in.ExtraCapabilities.CronCapabilityBinaryPath
		}

		if in.ExtraCapabilities.LogEventTriggerBinaryPath != "" || withPluginsDockerImageFlag != "" {
			workflowDONCapabilities = append(workflowDONCapabilities, cretypes.LogTriggerCapability)
			capabilitiesBinaryPaths[cretypes.LogTriggerCapability] = in.ExtraCapabilities.LogEventTriggerBinaryPath
		}

		for capabilityName, binaryPath := range extraBinaries {
			if binaryPath != "" || withPluginsDockerImageFlag != "" {
				workflowDONCapabilities = append(workflowDONCapabilities, capabilityName)
				capabilitiesBinaryPaths[capabilityName] = binaryPath
			}
		}

		capabiliitesDONCapabilities := []string{cretypes.WriteEVMCapability, cretypes.WebAPITargetCapability}
		if in.ExtraCapabilities.ReadContractBinaryPath != "" || withPluginsDockerImageFlag != "" {
			capabiliitesDONCapabilities = append(capabiliitesDONCapabilities, cretypes.ReadContractCapability)
			capabilitiesBinaryPaths[cretypes.ReadContractCapability] = in.ExtraCapabilities.ReadContractBinaryPath
		}

		capabilitiesAwareNodeSets = []*cretypes.CapabilitiesAwareNodeSet{
			{
				Input:              in.NodeSets[0],
				Capabilities:       workflowDONCapabilities,
				DONTypes:           []string{cretypes.WorkflowDON},
				BootstrapNodeIndex: 0,
			},
			{
				Input:              in.NodeSets[1],
				Capabilities:       capabiliitesDONCapabilities,
				DONTypes:           []string{cretypes.CapabilitiesDON}, // <----- it's crucial to set the correct DON type
				BootstrapNodeIndex: -1,                                 // <----- it's crucial to indicate there's no bootstrap node
			},
			{
				Input:              in.NodeSets[2],
				Capabilities:       []string{},
				DONTypes:           []string{cretypes.GatewayDON}, // <----- it's crucial to set the correct DON type
				BootstrapNodeIndex: -1,                            // <----- it's crucial to indicate there's no bootstrap node
				GatewayNodeIndex:   0,
			},
		}
	}

	// unset DockerFilePath and DockerContext as we cannot use them with existing images
	if withPluginsDockerImageFlag != "" {
		for setIdx := range capabilitiesAwareNodeSets {
			for nodeIdx := range capabilitiesAwareNodeSets[setIdx].NodeSpecs {
				capabilitiesAwareNodeSets[setIdx].NodeSpecs[nodeIdx].Node.Image = withPluginsDockerImageFlag
				capabilitiesAwareNodeSets[setIdx].NodeSpecs[nodeIdx].Node.DockerContext = ""
				capabilitiesAwareNodeSets[setIdx].NodeSpecs[nodeIdx].Node.DockerFilePath = ""
			}
		}
	}

	fmt.Print(libformat.PurpleText("DON topology:\n"))
	for _, nodeSet := range capabilitiesAwareNodeSets {
		fmt.Print(libformat.PurpleText("%s\n", strings.ToUpper(nodeSet.Name)))
		fmt.Print(libformat.PurpleText("\tNode count: %d\n", len(nodeSet.NodeSpecs)))
		capabilitiesDesc := "none"
		if len(nodeSet.Capabilities) > 0 {
			capabilitiesDesc = strings.Join(nodeSet.Capabilities, ", ")
		}
		fmt.Print(libformat.PurpleText("\tCapabilities: %s\n", capabilitiesDesc))
		fmt.Print(libformat.PurpleText("\tDON Types: %s\n\n", strings.Join(nodeSet.DONTypes, ", ")))
	}

	// add support for more capabilities if needed
	capabilityFactoryFns := []cretypes.DONCapabilityWithConfigFactoryFn{
		webapicap.WebAPITriggerCapabilityFactoryFn,
		webapicap.WebAPITargetCapabilityFactoryFn,
		computecap.ComputeCapabilityFactoryFn,
		consensuscap.OCR3CapabilityFactoryFn,
		croncap.CronCapabilityFactoryFn,
	}

	containerPath, pathErr := crecapabilities.DefaultContainerDirectory(in.Infra.InfraType)
	if pathErr != nil {
		return nil, fmt.Errorf("failed to get default container directory: %w", pathErr)
	}

	homeChainIDInt, chainErr := strconv.Atoi(in.Blockchains[0].ChainID)
	if chainErr != nil {
		return nil, fmt.Errorf("failed to convert chain ID to int: %w", chainErr)
	}

	cronBinaryName := filepath.Base(in.ExtraCapabilities.CronCapabilityBinaryPath)
	if withPluginsDockerImageFlag != "" {
		cronBinaryName = "cron"
	}

	logEventTriggerBinaryName := filepath.Base(in.ExtraCapabilities.LogEventTriggerBinaryPath)
	if withPluginsDockerImageFlag != "" {
		logEventTriggerBinaryName = "log-event-trigger"
	}

	readContractBinaryName := filepath.Base(in.ExtraCapabilities.ReadContractBinaryPath)
	if withPluginsDockerImageFlag != "" {
		readContractBinaryName = "readcontract"
	}

	jobSpecFactoryFunctions := []cretypes.JobSpecFactoryFn{
		// add support for more job spec factory functions if needed
		webapi.WebAPITriggerJobSpecFactoryFn,
		webapi.WebAPITargetJobSpecFactoryFn,
		creconsensus.ConsensusJobSpecFactoryFn(libc.MustSafeUint64(int64(homeChainIDInt))),
		crecron.CronJobSpecFactoryFn(filepath.Join(containerPath, cronBinaryName)),
		cregateway.GatewayJobSpecFactoryFn(extraAllowedGatewayPorts, []string{}, []string{"0.0.0.0/0"}),
		crecompute.ComputeJobSpecFactoryFn,
	}

	jobSpecFactoryFunctions = append(jobSpecFactoryFunctions, extraJobFactoryFns...)

	for _, blockchain := range in.Blockchains {
		chainIDInt, chainErr := strconv.Atoi(blockchain.ChainID)
		if chainErr != nil {
			return nil, fmt.Errorf("failed to convert chain ID to int: %w", chainErr)
		}

		if !blockchain.ReadOnly {
			capabilityFactoryFns = append(capabilityFactoryFns, writeevmcap.WriteEVMCapabilityFactory(libc.MustSafeUint64(int64(chainIDInt))))
		}
		capabilityFactoryFns = append(capabilityFactoryFns, readcontractcap.ReadContractCapabilityFactory(libc.MustSafeUint64(int64(chainIDInt)), "evm"))
		capabilityFactoryFns = append(capabilityFactoryFns, logeventtriggercap.LogEventTriggerCapabilityFactory(libc.MustSafeUint64(int64(chainIDInt)), "evm"))

		jobSpecFactoryFunctions = append(jobSpecFactoryFunctions, crelogevent.LogEventTriggerJobSpecFactoryFn(
			chainIDInt,
			"evm",
			// path within the container/pod
			filepath.Join(containerPath, logEventTriggerBinaryName),
		))

		jobSpecFactoryFunctions = append(jobSpecFactoryFunctions, crereadcontract.ReadContractJobSpecFactoryFn(
			chainIDInt,
			"evm",
			// path within the container/pod
			filepath.Join(containerPath, readContractBinaryName),
		))
	}

	if in.JD.CSAEncryptionKey == "" {
		// generate a new key
		key, keyErr := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
		if keyErr != nil {
			return nil, fmt.Errorf("failed to generate CSA encryption key: %w", keyErr)
		}
		in.JD.CSAEncryptionKey = hex.EncodeToString(crypto.FromECDSA(key)[:32])
		fmt.Printf("Generated new CSA encryption key for JD: %s\n", in.JD.CSAEncryptionKey)
	}
	universalSetupInput := creenv.SetupInput{
		CapabilitiesAwareNodeSets:            capabilitiesAwareNodeSets,
		CapabilitiesContractFactoryFunctions: capabilityFactoryFns,
		BlockchainsInput:                     in.Blockchains,
		JdInput:                              *in.JD,
		InfraInput:                           *in.Infra,
		JobSpecFactoryFunctions:              jobSpecFactoryFunctions,
		ConfigFactoryFunctions: []cretypes.ConfigFactoryFn{
			gatewayconfig.GenerateConfig,
		},
		S3ProviderInput: in.S3ProviderInput,
	}

	if withPluginsDockerImageFlag == "" {
		universalSetupInput.CustomBinariesPaths = capabilitiesBinaryPaths
	}

	ctx, cancel := context.WithTimeout(cmdContext, 10*time.Minute)
	defer cancel()
	universalSetupOutput, setupErr := creenv.SetupTestEnvironment(ctx, testLogger, cldlogger.NewSingleFileLogger(nil), universalSetupInput)
	if setupErr != nil {
		return nil, fmt.Errorf("failed to setup test environment: %w", setupErr)
	}

	return universalSetupOutput, nil
}

func isCRECLIIsAvailable() bool {
	if _, statErr := os.Stat(creCLI); statErr == nil {
		return true
	}

	pathCmd := exec.Command("which", creCLI)
	if err := pathCmd.Run(); err == nil {
		return true
	}

	return false
}

func tryToDownloadCRECLI() error {
	start := time.Now()
	fmt.Print(libformat.PurpleText("\n[Stage 2a/3] Downloading CRE CLI\n"))
	commandArgs := []string{"release", "download", "v0.2.0", "--repo", "smartcontractkit/dev-platform", "--pattern", "*darwin_arm64*", "--skip-existing"}

	ghCmd := exec.Command("gh", commandArgs...) // #nosec G204
	ghCmd.Stdout = os.Stdout
	ghCmd.Stderr = os.Stderr
	if startErr := ghCmd.Start(); startErr != nil {
		return errors.Wrap(startErr, "failed to start gh cli command")
	}

	if waitErr := ghCmd.Wait(); waitErr != nil {
		return errors.Wrap(waitErr, "failed to wait for gh cli command")
	}

	archiveName := "cre_v0.2.0_darwin_arm64.tar.gz"
	tarArgs := []string{"-xf", archiveName}
	tarCmd := exec.Command("tar", tarArgs...)

	tarCmd.Stdout = os.Stdout
	tarCmd.Stderr = os.Stderr
	if startErr := tarCmd.Start(); startErr != nil {
		return errors.Wrap(startErr, "failed to start tar command")
	}

	if waitErr := tarCmd.Wait(); waitErr != nil {
		return errors.Wrap(waitErr, "failed to wait for tar command")
	}

	removeErr := os.Remove(archiveName)
	if removeErr != nil {
		fmt.Fprintf(os.Stderr, "failed to remove %s. Please remove it manually.\n", archiveName)
	}

	fmt.Print(libformat.PurpleText("[Stage 2a/3] CRE CLI downloaded in %.2f seconds\n\n", time.Since(start).Seconds()))

	return nil
}

func creCLIAbsPath() (string, error) {
	var CRECLIAbsPath string

	_, statErr := os.Stat(creCLI)
	if statErr != nil {
		path, lookErr := exec.LookPath(creCLI)
		if lookErr != nil {
			return "", errors.Wrap(lookErr, "failed to find absolute path of the CRE CLI binary in PATH")
		}
		CRECLIAbsPath = path
	} else {
		var CRECLIAbsPathErr error
		CRECLIAbsPath, CRECLIAbsPathErr = filepath.Abs(creCLI)
		if CRECLIAbsPathErr != nil {
			return "", errors.Wrap(CRECLIAbsPathErr, "failed to find absolute path of the CRE CLI binary in current directory")
		}
	}

	return CRECLIAbsPath, nil
}

func isBlockscoutRunning(cmdContext context.Context) bool {
	dockerClient, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(cmdContext, 15*time.Second)
	defer cancel()
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return false
	}

	for _, container := range containers {
		if strings.Contains(strings.ToLower(container.Names[0]), "blockscout") {
			return true
		}
	}

	return false
}

func PrintCRELogo() {
	blue := "\033[38;5;33m"
	reset := "\033[0m"

	fmt.Println()
	fmt.Println(blue + "	db       .d88b.   .o88b.  .d8b.  db            .o88b. d8888b. d88888b" + reset)
	fmt.Println(blue + "	88      .8P  Y8. d8P  Y8 d8' `8b 88           d8P  Y8 88  `8D 88'" + reset)
	fmt.Println(blue + "	88      88    88 8P      88ooo88 88           8P      88oobY' 88ooooo" + reset)
	fmt.Println(blue + "	88      88    88 8b      88~~~88 88           8b      88`8b   88~~~~~" + reset)
	fmt.Println(blue + "	88booo. `8b  d8' Y8b  d8 88   88 88booo.      Y8b  d8 88 `88. 88." + reset)
	fmt.Println(blue + "	Y88888P  `Y88P'   `Y88P' YP   YP Y88888P       `Y88P' 88   YD Y88888P" + reset)
	fmt.Println()
}

func defaultCtfConfigs(topologyFlag string) error {
	if os.Getenv("CTF_CONFIGS") == "" {
		// use default config
		if topologyFlag == TopologySimplified {
			setErr := os.Setenv("CTF_CONFIGS", "configs/single-don.toml")
			if setErr != nil {
				return fmt.Errorf("failed to set CTF_CONFIGS environment variable: %w", setErr)
			}
		} else {
			setErr := os.Setenv("CTF_CONFIGS", "configs/workflow-capabilities-don.toml")
			if setErr != nil {
				return fmt.Errorf("failed to set CTF_CONFIGS environment variable: %w", setErr)
			}
		}
		fmt.Printf("Set CTF_CONFIGS environment variable to default value: %s\n", os.Getenv("CTF_CONFIGS"))
	}

	return nil
}

func hasBuiltDockerImage(in *Config, withPluginsDockerImageFlag string) bool {
	if withPluginsDockerImageFlag != "" {
		return false
	}

	hasBuilt := false

	for _, nodeset := range in.NodeSets {
		for _, nodeSpec := range nodeset.NodeSpecs {
			if nodeSpec.Node != nil && nodeSpec.Node.DockerFilePath != "" {
				hasBuilt = true
				break
			}
		}
	}

	return hasBuilt
}

func getCtfDockerNetworks() ([]string, error) {
	dockerClient, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Docker client")
	}
	defer dockerClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// List all containers with the "ctf" label
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "framework=ctf")),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list containers")
	}

	// Use a map to store unique network names
	networkMap := make(map[string]bool)
	var networkNames []string

	for _, container := range containers {
		// Get container details to access network settings
		containerDetails, err := dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			// Log the error but continue with other containers
			fmt.Fprintf(os.Stderr, "failed to inspect container %s: %s\n", container.ID, err)
			continue
		}

		// Extract network names from container's network settings
		for networkName := range containerDetails.NetworkSettings.Networks {
			if networkName == "bridge" {
				continue
			}
			if !networkMap[networkName] {
				networkMap[networkName] = true
				networkNames = append(networkNames, networkName)
			}
		}
	}

	return networkNames, nil
}
