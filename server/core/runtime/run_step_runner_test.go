package runtime_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	. "github.com/petergtz/pegomock/v4"
	"github.com/runatlantis/atlantis/server/core/config/valid"
	"github.com/runatlantis/atlantis/server/core/runtime"
	tf "github.com/runatlantis/atlantis/server/core/terraform"
	"github.com/runatlantis/atlantis/server/core/terraform/mocks"
	tfclientmocks "github.com/runatlantis/atlantis/server/core/terraform/tfclient/mocks"
	"github.com/runatlantis/atlantis/server/events/command"
	"github.com/runatlantis/atlantis/server/events/models"
	jobmocks "github.com/runatlantis/atlantis/server/jobs/mocks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestRunStepRunner_Run(t *testing.T) {
	cases := []struct {
		Command      string
		ProjectName  string
		ExpOut       string
		ExpErr       string
		Version      string
		Distribution string
	}{
		{
			Command: "",
			ExpOut:  "",
			Version: "v1.2.3",
		},
		{
			Command: "echo hi",
			ExpOut:  "hi\n",
			Version: "v2.3.4",
		},
		{
			Command: `printf \'your main.tf file does not provide default region.\\ncheck\'`,
			ExpOut:  "'your\n",
		},
		{
			Command: `printf 'your main.tf file does not provide default region.\ncheck'`,
			ExpOut:  "your main.tf file does not provide default region.\ncheck\n",
		},
		{
			Command: `printf '\e[32mgreen'`,
			ExpOut:  "green\n",
		},
		{
			Command: "echo 'a",
			ExpErr:  "exit status 2: running \"echo 'a\" in",
		},
		{
			Command: "echo hi >> file && cat file",
			ExpOut:  "hi\n",
		},
		{
			Command: "lkjlkj",
			ExpErr:  "exit status 127: running \"lkjlkj\" in",
		},
		{
			Command: "echo workspace=$WORKSPACE version=$ATLANTIS_TERRAFORM_VERSION dir=$DIR planfile=$PLANFILE showfile=$SHOWFILE project=$PROJECT_NAME",
			ExpOut:  "workspace=myworkspace version=0.11.0 dir=$DIR planfile=$DIR/myworkspace.tfplan showfile=$DIR/myworkspace.json project=\n",
		},
		{
			Command:     "echo workspace=$WORKSPACE version=$ATLANTIS_TERRAFORM_VERSION dir=$DIR planfile=$PLANFILE showfile=$SHOWFILE project=$PROJECT_NAME",
			ProjectName: "my/project/name",
			ExpOut:      "workspace=myworkspace version=0.11.0 dir=$DIR planfile=$DIR/my::project::name-myworkspace.tfplan showfile=$DIR/my::project::name-myworkspace.json project=my/project/name\n",
		},
		{
			Command:      "echo distribution=$ATLANTIS_TERRAFORM_DISTRIBUTION",
			ProjectName:  "my/project/name",
			ExpOut:       "distribution=terraform\n",
			Distribution: "terraform",
		},
		{
			Command:      "echo distribution=$ATLANTIS_TERRAFORM_DISTRIBUTION",
			ProjectName:  "my/project/name",
			ExpOut:       "distribution=tofu\n",
			Distribution: "opentofu",
		},
		{
			Command: "echo base_repo_name=$BASE_REPO_NAME base_repo_owner=$BASE_REPO_OWNER head_repo_name=$HEAD_REPO_NAME head_repo_owner=$HEAD_REPO_OWNER head_branch_name=$HEAD_BRANCH_NAME head_commit=$HEAD_COMMIT base_branch_name=$BASE_BRANCH_NAME pull_num=$PULL_NUM pull_url=$PULL_URL pull_author=$PULL_AUTHOR repo_rel_dir=$REPO_REL_DIR",
			ExpOut:  "base_repo_name=basename base_repo_owner=baseowner head_repo_name=headname head_repo_owner=headowner head_branch_name=add-feat head_commit=12345abcdef base_branch_name=main pull_num=2 pull_url=https://github.com/runatlantis/atlantis/pull/2 pull_author=acme repo_rel_dir=mydir\n",
		},
		{
			Command: "echo user_name=$USER_NAME",
			ExpOut:  "user_name=acme-user\n",
		}, {
			Command: "echo $PATH",
			ExpOut:  fmt.Sprintf("%s:%s\n", os.Getenv("PATH"), "/bin/dir"),
		},
		{
			Command: "echo args=$COMMENT_ARGS",
			ExpOut:  "args=-target=resource1,-target=resource2\n",
		},
	}
	for _, customPolicyCheck := range []bool{false, true} {
		for _, c := range cases {
			var projVersion *version.Version
			var err error

			projVersion, err = version.NewVersion("v0.11.0")

			if c.Version != "" {
				projVersion, err = version.NewVersion(c.Version)
				Ok(t, err)
			}

			Ok(t, err)

			projTFDistribution := "terraform"
			if c.Distribution != "" {
				projTFDistribution = c.Distribution
			}

			defaultVersion, _ := version.NewVersion("0.8")

			RegisterMockTestingT(t)
			terraform := tfclientmocks.NewMockClient()
			defaultDistribution := tf.NewDistributionTerraformWithDownloader(mocks.NewMockDownloader())
			When(terraform.EnsureVersion(Any[logging.SimpleLogging](), Any[tf.Distribution](), Any[*version.Version]())).
				ThenReturn(nil)

			logger := logging.NewNoopLogger(t)
			projectCmdOutputHandler := jobmocks.NewMockProjectCommandOutputHandler()
			tmpDir := t.TempDir()

			r := runtime.RunStepRunner{
				TerraformExecutor:       terraform,
				DefaultTFDistribution:   defaultDistribution,
				DefaultTFVersion:        defaultVersion,
				TerraformBinDir:         "/bin/dir",
				ProjectCmdOutputHandler: projectCmdOutputHandler,
			}
			t.Run(fmt.Sprintf("%s_CustomPolicyCheck=%v", c.Command, customPolicyCheck), func(t *testing.T) {
				ctx := command.ProjectContext{
					BaseRepo: models.Repo{
						Name:  "basename",
						Owner: "baseowner",
					},
					HeadRepo: models.Repo{
						Name:  "headname",
						Owner: "headowner",
					},
					Pull: models.PullRequest{
						Num:        2,
						URL:        "https://github.com/runatlantis/atlantis/pull/2",
						HeadBranch: "add-feat",
						HeadCommit: "12345abcdef",
						BaseBranch: "main",
						Author:     "acme",
					},
					User: models.User{
						Username: "acme-user",
					},
					Log:                   logger,
					Workspace:             "myworkspace",
					RepoRelDir:            "mydir",
					TerraformDistribution: &projTFDistribution,
					TerraformVersion:      projVersion,
					ProjectName:           c.ProjectName,
					EscapedCommentArgs:    []string{"-target=resource1", "-target=resource2"},
					CustomPolicyCheck:     customPolicyCheck,
				}
				out, err := r.Run(ctx, nil, c.Command, tmpDir, map[string]string{"test": "var"}, true, valid.PostProcessRunOutputShow)
				if c.ExpErr != "" {
					ErrContains(t, c.ExpErr, err)
					return
				}
				Ok(t, err)
				// Replace $DIR in the exp with the actual temp dir. We do this
				// here because when constructing the cases we don't yet know the
				// temp dir.
				expOut := strings.Replace(c.ExpOut, "$DIR", tmpDir, -1)
				Equals(t, expOut, out)

				terraform.VerifyWasCalledOnce().EnsureVersion(Eq(logger), NotEq(defaultDistribution), Eq(projVersion))
				terraform.VerifyWasCalled(Never()).EnsureVersion(Eq(logger), Eq(defaultDistribution), Eq(defaultVersion))

			})
		}
	}
}
