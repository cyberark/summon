#!/usr/bin/env groovy
// 'product-pipelines-shared-library' draws from DevOps/product-pipelines-shared-library repository.
// 'conjur-enterprise-sharedlib' draws from Conjur-Enterprise/jenkins-pipeline-library repository.
// Point to a branch of a shared library by appending @my-branch-name to the library name
@Library(['product-pipelines-shared-library', 'conjur-enterprise-sharedlib']) _

// Automated release, promotion and dependencies
properties([
  // Include the automated release parameters for the build
  release.addParams(),
  // Dependencies of the project that should trigger builds
  dependencies(['conjur-enterprise/conjur-api-go'])
])

// Performs release promotion.  No other stages will be run
if (params.MODE == "PROMOTE") {
  release.promote(params.VERSION_TO_PROMOTE) { infrapool, sourceVersion, targetVersion, assetDirectory ->
    // Any assets from sourceVersion Github release are available in assetDirectory
    // Any version number updates from sourceVersion to targetVersion occur here
    // Any publishing of targetVersion artifacts occur here
    // Anything added to assetDirectory will be attached to the Github Release

    //Sign *tar.gz, *.deb, *.exe and summon_darwin_* artifacts
    infrapool.agentGet from: "${assetDirectory}/", to: "."

    signArtifacts patterns: ["*.tar.gz"]
    signArtifacts patterns: ["*.deb"]
    signArtifacts patterns: ["*.exe"]
    signArtifacts patterns: ["summon_darwin_*"]

    infrapool.agentPut from: "*.sig", to: "${assetDirectory}"
    infrapool.agentPut from: "*.deb", to: "${assetDirectory}"
    infrapool.agentPut from: "*.exe", to: "${assetDirectory}"
    infrapool.agentPut from: "summon_darwin_*", to: "${assetDirectory}"

    // Resolve ownership issue before promotion
    sh 'git config --global --add safe.directory ${PWD}'
  }

  release.copyEnterpriseRelease(params.VERSION_TO_PROMOTE)
  return
}

pipeline {
  agent { label 'conjur-enterprise-common-agent' }

  options {
    timestamps()
    buildDiscarder(logRotator(numToKeepStr: '30'))
  }

  environment {
    // Sets the MODE to the specified or autocalculated value as appropriate
    MODE = release.canonicalizeMode()
    // CI pulls Conjur images from the internal registry (not Docker Hub). bin/test.sh
    // maps INFRAPOOL_REGISTRY_URL → REGISTRY_URL on infrapool agents (SendEnv=INFRAPOOL_*).
    INFRAPOOL_REGISTRY_URL = 'registry.tld'
  }

  triggers {
    parameterizedCron("""
      ${getWeeklyCronString("H(0,6)", "%TEST_AZURE=true;TEST_GCP=true;TEST_SAAS=true")}
      ${getWeeklyCronString("H(1-5)", "%MODE=RELEASE;TEST_AZURE=true;TEST_GCP=true;TEST_SAAS=true")}
    """)
  }

  parameters {
    booleanParam(name: 'TEST_AWS', defaultValue: true, description: 'Run integration tests against AWS IAM')
    booleanParam(name: 'TEST_AZURE', defaultValue: false, description: 'Run integration tests against Azure')
    booleanParam(name: 'TEST_GCP', defaultValue: false, description: 'Run integration tests against GCP')
    booleanParam(name: 'TEST_CERT', defaultValue: false, description: 'Run authn-cert integration tests against Conjur Enterprise (auto-enabled on main/master)')
    booleanParam(name: 'TEST_SAAS', defaultValue: false, description: 'Run Secrets Manager SaaS JWT tests')
  }

  stages {
    // Aborts any builds triggered by another project that wouldn't include any changes
    stage ("Skip build if triggering job didn't create a release") {
      when {
        expression {
          MODE == "SKIP"
        }
      }
      steps {
        script {
          currentBuild.result = 'ABORTED'
          error("Aborting build because this build was triggered from upstream, but no release was built")
        }
      }
    }

    stage('Scan for internal URLs') {
      steps {
        script {
          detectInternalUrls()
        }
      }
    }

    stage('Get InfraPool ExecutorV2 Agent') {
      steps {
        script {
          // Request ExecutorV2 agents for 1 hour(s)
          infrapool = getInfraPoolAgent.connected(type: "ExecutorV2", quantity: 1, duration: 1)[0]

          // Request additional executors for cloud specific tests
          if (params.TEST_AZURE) {
            INFRAPOOL_AZURE_EXECUTORV2_AGENT_0 = getInfraPoolAgent.connected(type: "AzureExecutorV2", quantity: 1, duration: 1)[0]
          }
          if (params.TEST_GCP) {
            INFRAPOOL_GCP_EXECUTORV2_AGENT_0 = getInfraPoolAgent.connected(type: "GcpExecutorV2", quantity: 1, duration: 1)[0]
          }
        }
      }
    }

    stage('Validate Changelog') {
      steps { parseChangelog(infrapool) }
    }

    // Generates a VERSION file based on the current build number and latest version in CHANGELOG.md
    stage('Validate changelog and set version') {
      steps {
        script {
          updateVersion(infrapool, "CHANGELOG.md", "${BUILD_NUMBER}")

          if (params.TEST_AZURE) {
            updateVersion(INFRAPOOL_AZURE_EXECUTORV2_AGENT_0, "CHANGELOG.md", "${BUILD_NUMBER}")
          }
          if (params.TEST_GCP) {
            updateVersion(INFRAPOOL_GCP_EXECUTORV2_AGENT_0, "CHANGELOG.md", "${BUILD_NUMBER}")
          }
        }
      }
    }

    stage('Get latest upstream dependencies') {
      steps {
        script {
          updatePrivateGoDependencies("${WORKSPACE}/go.mod")
          // Copy the vendor directory onto infrapool
          infrapool.agentPut from: "vendor", to: "${WORKSPACE}"
          infrapool.agentPut from: "go.*", to: "${WORKSPACE}"
          infrapool.agentPut from: "/root/go", to: "/var/lib/jenkins/"
        }
      }
    }

    stage('Run unit tests') {
      steps {
        script {
          try {
            infrapool.agentSh './test_unit'
          } finally {
            infrapool.agentSh 'mv output/c.out . || true'
            infrapool.agentStash name: 'output-xml', includes: 'output/*.xml', allowEmpty: true
          }
          unstash 'output-xml'
          codacy action: 'reportCoverage', filePath: "output/coverage.xml"
        }
      }
      post {
        always {
          script {
            try {
              unstash 'output-xml'
            } catch (err) {
              echo "Skipping unit test report unstash: ${err.message}"
            }

            if (fileExists('output/junit.xml')) {
              junit 'output/junit.xml'
            } else {
              echo 'Skipping unit junit publication: output/junit.xml not found'
            }

            if (fileExists('output/coverage.xml')) {
              recordCoverage(
                      tools: [[parser: 'COBERTURA', pattern: 'output/coverage.xml']],
                      sourceCodeEncoding: 'ASCII',
                      qualityGates: [
                          [threshold: 74.0, metric: 'LINE', baseline: 'PROJECT', unstable: true],
                          [threshold: 90.0, metric: 'METHOD', baseline: 'PROJECT', unstable: true]
                      ],
                      skipPublishingChecks: false
                )
            } else {
              echo 'Skipping unit coverage publication: output/coverage.xml not found'
            }
          }
        }
      }
    }

    stage('Build Release Artifacts') {
      steps {
        script {
          infrapool.agentDir('./pristine-checkout') {
            // Go releaser requires a pristine checkout
            checkout scm

            // Copy the checkout content onto infrapool
            infrapool.agentPut from: "./", to: "."

            // Copy VERSION info into prisitine folder
            infrapool.agentSh "cp ../VERSION ./VERSION"

            infrapool.agentSh './build --snapshot'
            infrapool.agentArchiveArtifacts artifacts: 'pristine-checkout/dist/goreleaser/'
          }
        }
      }
    }

    stage('Run integration tests') {
      environment {
        INFRAPOOL_TEST_AWS = "${params.TEST_AWS}"
      }
      steps {
        script {
          def mainline = env.BRANCH_NAME == 'main' || env.BRANCH_NAME == 'master'
          def runCert = mainline || params.TEST_CERT
          env.INFRAPOOL_TEST_CERT = "${runCert}"
          env.TEST_CERT = "${runCert}"
          def testCmd = params.TEST_AWS ? 'summon -e aws ./bin/test.sh' : './bin/test.sh'
          infrapool.agentDir('./pristine-checkout') {
            try {
              infrapool.agentSh testCmd
            } finally {
              infrapool.agentStash name: 'integration-output', includes: 'output/integration/*.xml', allowEmpty: true
            }
          }
        }
      }
      post {
        always {
          script {
            try {
              unstash 'integration-output'
            } catch (err) {
              echo "Skipping integration test report unstash: ${err.message}"
            }

            if (fileExists('output/integration/junit.xml')) {
              junit allowEmptyResults: true, testResults: 'output/integration/*.xml'
            } else {
              echo 'Skipping integration junit publication: output/integration/junit.xml not found'
            }
          }
        }
      }
    }

    stage('Run Secrets Manager SaaS tests') {
      when {
        expression {
          return params.TEST_SAAS
        }
      }
      stages {
        stage('Create a Tenant') {
          steps {
            script {
              TENANT = getConjurCloudTenant()
            }
          }
        }
        stage('Authenticate') {
          steps {
            script {
              def id_token = getConjurCloudTenant.tokens(
                infrapool: infrapool,
                identity_url: "${TENANT.identity_information.idaptive_tenant_fqdn}",
                username: "${TENANT.login_name}"
              )

              def conj_token = getConjurCloudTenant.tokens(
                infrapool: infrapool,
                conjur_url: "${TENANT.conjur_cloud_url}",
                identity_token: "${id_token}"
              )

              env.identity_token = id_token
              env.conj_token = conj_token
            }
          }
        }
        stage('Run JWT tests against Tenant') {
          environment {
            INFRAPOOL_CONJUR_APPLIANCE_URL = "${TENANT.conjur_cloud_url}"
            INFRAPOOL_CONJUR_AUTHN_LOGIN = "${TENANT.login_name}"
            INFRAPOOL_CONJUR_AUTHN_TOKEN = "${env.conj_token}"
            INFRAPOOL_IDENTITY_TOKEN = "${env.identity_token}"
            INFRAPOOL_TEST_SAAS = true
          }
          steps {
            script {
              infrapool.agentDir('./pristine-checkout') {
                try {
                  infrapool.agentSh './bin/test.sh'
                } finally {
                  infrapool.agentStash name: 'saas-integration-output', includes: 'output/integration/*.xml', allowEmpty: true
                }
              }
            }
          }
        }
      }
      post {
        always {
          catchError(message: 'saas-integration-output stash not found', buildResult: 'SUCCESS') {
            unstash 'saas-integration-output'
          }
          junit allowEmptyResults: true, testResults: 'output/integration/*.xml'
          script {
            deleteConjurCloudTenant("${TENANT.id}")
          }
        }
      }
    }

    stage('Run cloud and acceptance tests') {
      parallel {
        stage('Run Azure tests') {
          when {
            expression { params.TEST_AZURE }
          }
          environment {
            INFRAPOOL_TEST_AZURE = true
            INFRAPOOL_REGISTRY_URL = 'registry.tld'
          }
          steps {
            script {
              INFRAPOOL_AZURE_EXECUTORV2_AGENT_0.agentSh './build --snapshot'
              INFRAPOOL_AZURE_EXECUTORV2_AGENT_0.agentSh 'summon -e azure ./bin/test.sh'
            }
          }
        }

        stage('Run GCP tests') {
          when {
            expression { params.TEST_GCP }
          }
          environment {
            INFRAPOOL_TEST_GCP = true
            INFRAPOOL_REGISTRY_URL = 'registry.tld'
          }
          steps {
            script {
              INFRAPOOL_GCP_EXECUTORV2_AGENT_0.agentSh "./bin/get_gcp_token.sh host/test-app cucumber gcp"
              INFRAPOOL_GCP_EXECUTORV2_AGENT_0.agentStash name: 'token-out', includes: "gcp/*"
              infrapool.agentUnstash name: 'token-out'
              infrapool.agentSh 'cp -r gcp ./pristine-checkout/'
              infrapool.agentDir('./pristine-checkout') {
                infrapool.agentSh './bin/test.sh'
              }
            }
          }
        }

        stage('Run acceptance tests') {
          steps {
            script {
              // Use the artifacts from the previous stage
              infrapool.agentDir('./pristine-checkout') {
                try {
                  infrapool.agentSh './test_acceptance'
                } finally {
                  infrapool.agentStash name: 'acceptance-output', includes: 'output/acceptance/*.xml', allowEmpty: true
                }
              }
            }
          }
          post {
            always {
              script {
                try {
                  unstash 'acceptance-output'
                } catch (err) {
                  echo "Skipping acceptance test report unstash: ${err.message}"
                }

                if (fileExists('output/acceptance/junit.xml')) {
                  junit 'output/acceptance/*.xml'
                } else {
                  echo 'Skipping acceptance junit publication: output/acceptance/*.xml not found'
                }
              }
            }
          }
        }
      }
    }

    stage('Release') {
      when {
        expression {
          MODE == "RELEASE"
        }
      }
      steps {
        script {
          release(infrapool) { billOfMaterialsDirectory, assetDirectory, toolsDirectory ->
            // Publish release artifacts to all the appropriate locations
            // Copy any artifacts to assetDirectory to attach them to the Github release

            // Copy assets to be published in Github release.
            infrapool.agentSh "${toolsDirectory}/bin/copy_goreleaser_artifacts ${assetDirectory}"

            // Create Go application SBOM using the go.mod version for the golang container image
            infrapool.agentSh """export PATH="${toolsDirectory}/bin:${PATH}" && go-bom --tools "${toolsDirectory}" --go-mod ./go.mod --image "golang" --main "cmd/" --output "${billOfMaterialsDirectory}/go-app-bom.json" """
            // Create Go module SBOM
            infrapool.agentSh """export PATH="${toolsDirectory}/bin:${PATH}" && go-bom --tools "${toolsDirectory}" --go-mod ./go.mod --image "golang" --output "${billOfMaterialsDirectory}/go-mod-bom.json" """
          }
        }
      }
    }

    stage('Validate installation script') {
      parallel {
        stage('Validate installation on Ubuntu 24.04') {
          steps {
            script {
              infrapool.agentSh 'bin/installer-test --ubuntu-24.04'
            }
          }
        }
        stage('Validate installation on Ubuntu 22.04') {
          steps {
            script {
              infrapool.agentSh 'bin/installer-test --ubuntu-22.04'
            }
          }
        }
      }
    }
  }

  post {
    always {
      releaseInfraPoolAgent(".infrapool/release_agents")
    }
  }
}
