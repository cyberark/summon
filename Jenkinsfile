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
  dependencies([])
])

// Performs release promotion.  No other stages will be run
if (params.MODE == "PROMOTE") {
  release.promote(params.VERSION_TO_PROMOTE) { infrapool, sourceVersion, targetVersion, assetDirectory ->
    // Any assets from sourceVersion Github release are available in assetDirectory
    // Any version number updates from sourceVersion to targetVersion occur here
    // Any publishing of targetVersion artifacts occur here
    // Anything added to assetDirectory will be attached to the Github Release
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

  triggers {
    cron(getDailyCronString())
  }

  environment {
    // Sets the MODE to the specified or autocalculated value as appropriate
    MODE = release.canonicalizeMode()
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
        }
      }
    }

    stage('Validate Changelog') {
      steps { parseChangelog(infrapool) }
    }

    // Generates a VERSION file based on the current build number and latest version in CHANGELOG.md
    stage('Validate changelog and set version') {
      steps {
        updateVersion(infrapool, "CHANGELOG.md", "${BUILD_NUMBER}")
      }
    }

    stage('Run unit tests') {
      steps {
        script {
          infrapool.agentSh './test_unit'
          infrapool.agentSh 'mv output/c.out .'
          infrapool.agentStash name: 'output-xml', includes: 'output/*.xml'
          unstash 'output-xml'
          codacy action: 'reportCoverage', filePath: "output/coverage.xml"
        }
      }
      post {
        always {
          unstash 'output-xml'
          junit 'output/junit.xml'
          cobertura autoUpdateHealth: false, autoUpdateStability: false, coberturaReportFile: 'output/coverage.xml', conditionalCoverageTargets: '100, 0, 0', failUnhealthy: true, failUnstable: false, lineCoverageTargets: '74, 0, 0', maxNumberOfBuilds: 0, methodCoverageTargets: '92, 0, 0', onlyStable: false, sourceEncoding: 'ASCII', zoomCoverageChart: false
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
            infrapool.agentArchiveArtifacts artifacts: 'dist/goreleaser/'
          }
        }
      }
    }

    stage('Run acceptance tests') {
      steps {
        script {
          // Use the artifacts from the previous stage
          infrapool.agentDir('./pristine-checkout') {
            infrapool.agentSh './test_acceptance'
            infrapool.agentStash name: 'acceptance-output', includes: 'output/acceptance/*.xml'
          }
        }
      }
      post {
        always {
          unstash 'acceptance-output'
          junit 'output/acceptance/*.xml'
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
        stage('Validate installation on Ubuntu 20:04') {
          steps {
            script {
              infrapool.agentSh 'bin/installer-test --ubuntu-20.04'
            }
          }
        }
        stage('Validate installation on Ubuntu 18:04') {
          steps {
            script {
              infrapool.agentSh 'bin/installer-test --ubuntu-18.04'
            }
          }
        }
        stage('Validate installation on Ubuntu 16:04') {
          steps {
            script {
              infrapool.agentSh 'bin/installer-test --ubuntu-16.04'
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
