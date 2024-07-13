package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alexellis/k3sup/pkg"
	"github.com/spf13/cobra"
)

func MakePlan() *cobra.Command {
	var initFlag bool

	var command = &cobra.Command{
		Use:   "plan",
		Short: "Plan an installation of K3s.",
		Long: `Generate a bash script or plan of installation commands for K3s for a 
Highly Available (HA) Kubernetes cluster.

Examples JSON input file:

[{"hostname": "node-1", "ip": "192.168.128.102"},
{"hostname": "node-2", "ip": "192.168.128.103"},
{"hostname": "node-3", "ip": "192.168.128.104"}]

` + pkg.SupportMessageShort + `
`,
		Example: `  # Generate an installation script where the first
  # 3 available hosts are dedicated as servers, with a custom user.
  # The remaining hosts are added as agents.
  k3sup plan hosts.json --servers 3 --user ubuntu

  # Override the TLS SAN, for HA with 5 servers specified
  k3sup plan hosts.json --servers 5 --tls-san $SAN_IP

  # Create an example hosts.json file
  k3sup plan --init
`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if initFlag {
				return outputExampleHostsJSON()
			}

			if len(args) == 0 {
				return fmt.Errorf("give a path to a JSON file containing a list of devices")
			}

			nodeLimit, _ := cmd.Flags().GetInt("limit")
			name := args[0]
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}

			background, _ := cmd.Flags().GetBool("background")

			var hosts []Host
			if err = json.Unmarshal(data, &hosts); err != nil {
				return err
			}

			serverK3sExtraArgs, _ := cmd.Flags().GetString("server-k3s-extra-args")
			agentK3sExtraArgs, _ := cmd.Flags().GetString("agent-k3s-extra-args")

			servers, _ := cmd.Flags().GetInt("servers")
			kubeconfig, _ := cmd.Flags().GetString("local-path")
			contextName, _ := cmd.Flags().GetString("context")
			user, _ := cmd.Flags().GetString("user")
			tlsSan, _ := cmd.Flags().GetString("tls-san")

			tlsSanStr := ""
			if len(tlsSan) > 0 {
				tlsSanStr = fmt.Sprintf(` \
--tls-san %s`, tlsSan)
			}
			// sshKey, _ := cmd.Flags().GetString("ssh-key")

			bgStr := ""
			if background {
				bgStr = " &"
			}

			serversAdded := 0
			var primaryServer Host
			script := "#!/bin/sh\n\n"

			serverExtraArgsSt := ""
			if len(serverK3sExtraArgs) > 0 {
				serverExtraArgsSt = fmt.Sprintf(` \
--k3s-extra-args "%s"`, serverK3sExtraArgs)
			}
			agentExtraArgsSt := ""
			if len(agentK3sExtraArgs) > 0 {
				agentExtraArgsSt = fmt.Sprintf(` \
--k3s-extra-args "%s"`, agentK3sExtraArgs)
			}

			for i, host := range hosts {
				if serversAdded == 0 {

					script += `echo "Setting up primary server 1"
`

					script += fmt.Sprintf(`k3sup install --host %s \
--user %s \
--cluster \
--local-path %s \
--context %s%s%s
`,
						host.IP,
						user,
						kubeconfig,
						contextName,
						tlsSanStr,
						serverExtraArgsSt)

					script += fmt.Sprintf(`
echo "Fetching the server's node-token into memory"

export NODE_TOKEN=$(k3sup node-token --host %s --user %s)
`, host.IP, user)

					serversAdded = 1
					primaryServer = host
				} else if serversAdded < servers {
					script += fmt.Sprintf("\necho \"Setting up additional server: %d\"\n", serversAdded+1)

					script += fmt.Sprintf(`k3sup join \
--host %s \
--server-host %s \
--server \
--node-token "$NODE_TOKEN" \
--user %s%s%s%s
`, host.IP, primaryServer.IP, user, tlsSanStr, serverExtraArgsSt, bgStr)

					serversAdded++
				} else {
					script += fmt.Sprintf("\necho \"Setting up worker: %d\"\n", (i+1)-serversAdded)

					script += fmt.Sprintf(`k3sup join \
--host %s \
--server-host %s \
--node-token "$NODE_TOKEN" \
--user %s%s%s
`, host.IP, primaryServer.IP, user, agentExtraArgsSt, bgStr)
				}

				if nodeLimit > 0 && i+1 >= nodeLimit {
					break
				}
			}

			fmt.Printf("%s\n", script)

			return nil
		},
	}

	command.Flags().Int("servers", 3, "Number of servers to use from the devices file")
	command.Flags().String("local-path", "kubeconfig", "Where to save the kubeconfig file")
	command.Flags().String("context", "default", "Name of the kubeconfig context to use")
	command.Flags().String("user", "root", "Username for SSH login")

	command.Flags().String("ssh-key", "", "Path to the private key for SSH login")
	command.Flags().String("tls-san", "", "SAN for TLS certificates, can be a comma-separated list")
	command.Flags().String("server-k3s-extra-args", "", "Extra arguments to be passed into the k3s server")
	command.Flags().String("agent-k3s-extra-args", "", "Extra arguments to be passed into the k3s agent")

	// Background
	command.Flags().Bool("background", false, "Run the installation in the background for all agents/nodes after the first server is up")

	command.Flags().Int("limit", 0, "Maximum number of nodes to use from the devices file, 0 to use all devices")
	command.Flags().BoolVar(&initFlag, "init", false, "Output an example hosts.json file")

	return command
}

func outputExampleHostsJSON() error {
	exampleHosts := []Host{
		{Hostname: "node-1", IP: "192.168.128.102"},
		{Hostname: "node-2", IP: "192.168.128.103"},
		{Hostname: "node-3", IP: "192.168.128.104"},
	}

	data, err := json.MarshalIndent(exampleHosts, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode JSON: %v", err)
	}

	fmt.Println(string(data))
	return nil
}

type Host struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}
