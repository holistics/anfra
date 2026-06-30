package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anfra-ai/anfra/internal/project"
	"github.com/anfra-ai/anfra/internal/query"
	"github.com/anfra-ai/anfra/internal/sidecar"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newQueryCmd() *cobra.Command {
	var (
		generate bool
		aqlFlag  string
		dataset  string
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Compile and run an AQL query against a dataset",
		RunE: func(_ *cobra.Command, _ []string) error {
			if dataset == "" {
				return errors.New("--dataset is required")
			}
			aql, err := resolveAQL(aqlFlag)
			if err != nil {
				return err
			}

			if generate {
				return withSidecar(func(ctx context.Context, node *sidecar.AnfraNodeClient, proj project.Project) error {
					sql, err := query.GenerateSQL(ctx, node, proj, dataset, aql)
					if err != nil {
						return err
					}
					fmt.Println(sql)
					return nil
				})
			}

			// Execution needs both anfra-node (compile) and canal-query (run) sidecars.
			return withProject(func(h hostContext) error {
				node := sidecar.NewAnfraNode(h.cfg)
				if err := node.Start(h.ctx); err != nil {
					return fmt.Errorf("start anfra-node sidecar: %w", err)
				}
				defer node.Close()

				canalQuery := sidecar.NewCanalQuery(h.cfg)
				if err := canalQuery.Start(h.ctx); err != nil {
					return fmt.Errorf("start canal-query sidecar: %w", err)
				}
				defer canalQuery.Close()

				result, err := query.Run(h.ctx, node.Client(), canalQuery.Client(), h.proj, dataset, aql)
				if err != nil {
					return err
				}
				return printResult(result)
			})
		},
	}
	cmd.Flags().BoolVar(&generate, "generate", false, "output the generated SQL instead of running the query")
	cmd.Flags().StringVar(&aqlFlag, "aql", "", "the AQL query; if omitted, read from stdin")
	cmd.Flags().StringVar(&dataset, "dataset", "", "unique name of the dataset to compile against (required)")
	return cmd
}

// resolveAQL takes the query from --aql, or from stdin when it's piped, so
// `cat query.aql | anfra query --dataset x` works. Errors rather than blocking
// when neither is provided.
func resolveAQL(flag string) (string, error) {
	if strings.TrimSpace(flag) != "" {
		return flag, nil
	}
	stat, _ := os.Stdin.Stat()
	if stat != nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		return "", errors.New("no AQL provided: pass --aql or pipe it via stdin")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read AQL from stdin: %w", err)
	}
	aql := strings.TrimSpace(string(data))
	if aql == "" {
		return "", errors.New("no AQL provided: pass --aql or pipe it via stdin")
	}
	return aql, nil
}

// queryOutput is the YAML shape printed for an executed query.
type queryOutput struct {
	SQL    string `yaml:"sql"`
	Result struct {
		Fields  []string `yaml:"fields"`
		Records [][]any  `yaml:"records"`
	} `yaml:"result"`
}

func printResult(r *query.RunResult) error {
	var out queryOutput
	out.SQL = r.SQL
	out.Result.Fields = r.Fields
	out.Result.Records = r.Records
	b, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Print(string(b))
	return nil
}
