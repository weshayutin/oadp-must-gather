package pkg

import (
	"fmt"
	"os"
	"time"

	openshiftconfigv1 "github.com/openshift/api/config/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/mateusoliveira43/oadp-must-gather/pkg/gather"
	"github.com/mateusoliveira43/oadp-must-gather/pkg/templates"
)

// TODO <this-image> const

// TODO which errors should make must-gather exit earlier?

var (
	LogsSince time.Duration
	Timeout   time.Duration
	SkipTLS   bool
	// essentialOnly bool

	CLI = &cobra.Command{
		Use: "oc adm must-gather --image=<this-image> -- /usr/bin/gather",
		Long: `OADP Must-gather

TODO`,
		Args: cobra.NoArgs,
		Example: `  # TODO
  oc adm must-gather --image=<this-image>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --essential-only --logs-since <time>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --timeout <time>

  # TODO
  oc adm must-gather --image=<this-image> -- /usr/bin/gather --skip-tls --timeout <time>

  # TODO metrics dump`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			// TODO test flags
			// fmt.Printf("logsSince %#v\n", LogsSince)

			clusterConfig := config.GetConfigOrDie()
			// TODO https://github.com/deads2k/oc/blob/46db7c2bce5a57e3c3d9347e7e1e107e61dbd306/pkg/cli/admin/inspect/inspect.go#L142
			clusterConfig.QPS = 999999
			clusterConfig.Burst = 999999

			clusterClient, err := client.New(clusterConfig, client.Options{})
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while creating Go client: %v\n", err)
				return err
			}

			// TODO check error?
			openshiftconfigv1.AddToScheme(clusterClient.Scheme())
			operatorsv1alpha1.AddToScheme(clusterClient.Scheme())
			storagev1.AddToScheme(clusterClient.Scheme())
			corev1.AddToScheme(clusterClient.Scheme())

			clusterVersion, err := gather.ClusterVersion(clusterClient)
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while gathering ClusterVersion: %v\n", err)
				return err
			}
			// TODO why truncate???
			clusterID := string(clusterVersion.Spec.ClusterID[:8])

			// for now, lest keep the folder structure as it is
			//     must-gather/clusters/<id>/cluster-scoped-resources/apiextensions.k8s.io/customresourcedefinitions
			//     must-gather/clusters/<id>/namespaces/<name>/velero.io/<name>
			//     must-gather/clusters/<id>/namespaces/<name>/oadp.openshift.io/<name>
			// otherwise may break `omg` usage. ref https://github.com/openshift/oadp-operator/pull/1269
			path := fmt.Sprintf("must-gather/clusters/%s", clusterID)
			// TODO be careful about DUPLICATION when creating the folders
			// TODO permission
			// TODO need defer somewhere?
			err = os.MkdirAll(path+"/cluster-scoped-resources/storage.k8s.io", 0777)
			if err != nil {
				fmt.Printf("Exiting OADP must-gather, an error happened while creating folder structure: %v\n", err)
				return err
			}

			// do this part in parallel? --------------------------------------
			infrastructure, err := gather.Infrastructure(clusterClient)
			if err != nil {
				fmt.Println(err)
			}

			nodeList, err := gather.ALLNodes(clusterClient)
			if err != nil {
				fmt.Println(err)
			}
			// ----------------------------------------------------------------

			// do this part in parallel? --------------------------------------
			// get namespaces with OADP installs
			clusterServiceVersionList, err := gather.AllClusterServiceVersions(clusterClient)
			if err != nil {
				fmt.Println(err)
			}
			// ----------------------------------------------------------------
			// TODO Collect all OADP/Velero CRDs
			// TODO Collect all OADP/Velero CRs in all namespaces
			// TODO when Velero/OADP API updates, how to handle? use dynamic client instead?

			// oc adm inspect --dest-dir must-gather/clusters/${clusterID} --all-namespaces ns/${ns}

			// gather_logs

			// gather_metrics
			// Find problem with velero metrics (port?) and kill html, add to summary.md file

			// gather_versions https://github.com/openshift/oadp-operator/pull/994
			// do this part in parallel? --------------------------------------
			storageClassList, err := gather.AllStorageClasses(clusterClient)
			if err != nil {
				fmt.Println(err)
			}
			// ----------------------------------------------------------------

			// TODO do processes in parallel!?
			templates.ReplaceClusterInformationSection(clusterID, clusterVersion, infrastructure, nodeList)
			templates.ReplaceOADPOperatorInstallationSection(clusterServiceVersionList)
			templates.ReplaceAvailableStorageClassesSection(path, storageClassList)
			// do not tar!
			err = templates.Write(path)
			if err != nil {
				fmt.Printf("Error occurred: %v\n", err)
				return err
			}
			return nil
		},
	}
)
