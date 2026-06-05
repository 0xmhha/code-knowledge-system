// Command cks-domain-export renders a project's domain-knowledge entries
// (status verified/needs_verification) plus its authoritative_docs into a
// markdown corpus that `ckv build --docs <out>` embeds. This is the
// producer side of channel ②.
//
// Usage:
//
//	cks-domain-export -project docs/domain-knowledge/projects/go-stablenet \
//	  -out generated/domain-corpus/go-stablenet
//
// code_root for authoritative_docs resolves via CKS_CODE_ROOT or the
// project.yaml ${GO_STABLENET_ROOT} env, same as the validator.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xmhha/code-knowledge-system/internal/domainexport"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	projectDir := flag.String("project", "", "project directory (contains project.yaml, subsystems.yaml, entries/)")
	outDir := flag.String("out", "", "output corpus directory")
	flag.Parse()

	if *projectDir == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "cks-domain-export: -project and -out are required")
		flag.Usage()
		os.Exit(2)
	}

	p, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-domain-export: %v\n", err)
		os.Exit(1)
	}
	res, err := domainexport.Export(p, *outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-domain-export: %v\n", err)
		os.Exit(1)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "cks-domain-export: warning: %s\n", w)
	}
	fmt.Printf("cks-domain-export: %d entries, %d docs -> %s\n", res.EntriesWritten, res.DocsCopied, *outDir)
}
