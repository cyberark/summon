#!/usr/bin/env groovy
@Library("product-pipelines-shared-library") _

pipeline {
  agent { label 'conjur-enterprise-common-agent' }

  options {
    timestamps()
    buildDiscarder(logRotator(numToKeepStr: '30'))
    skipDefaultCheckout()  // see 'post' below, once perms are fixed this is no longer needed
  }

  triggers {
    cron(getDailyCronString())
  }

  stages {
    stage('Checkout SCM') {
      steps {
        checkout scm
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
          INFRAPOOL_EXECUTORV2_AGENT_0 = getInfraPoolAgent.connected(type: "ExecutorV2", quantity: 1, duration: 1)[0]
        }
      }
    }

    stage('Validate Changelog') {
      steps { script { INFRAPOOL_EXECUTORV2_AGENT_0.agentSh './bin/parse-changelog' } }
    }

    stage('Run unit tests') {
      steps {
        script {
          INFRAPOOL_EXECUTORV2_AGENT_0.agentSh './test_unit'
          INFRAPOOL_EXECUTORV2_AGENT_0.agentSh 'mv output/c.out .'
          INFRAPOOL_EXECUTORV2_AGENT_0.agentStash name: 'output-xml', includes: 'output/*.xml'
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
      when {
        not { buildingTag() }
      }

      steps {
        script {
          INFRAPOOL_EXECUTORV2_AGENT_0.agentSh './build --snapshot'
          INFRAPOOL_EXECUTORV2_AGENT_0.agentArchiveArtifacts artifacts: 'dist/goreleaser/'
        }
      }
    }

    stage('Build Release Artifacts and Create Pre Release') {
      // Only run this stage when triggered by a tag
      when { buildingTag() }

      steps {
        script {
          INFRAPOOL_EXECUTORV2_AGENT_0.agentDir('./pristine-checkout') {
            // Go releaser requires a pristine checkout
            checkout scm

            // Copy the checkout content onto infrapool
            INFRAPOOL_EXECUTORV2_AGENT_0.agentPut from: "./", to: "."

            // Create draft release
            INFRAPOOL_EXECUTORV2_AGENT_0.agentSh 'summon --yaml "GITHUB_TOKEN: !var github/users/conjur-jenkins/api-token" ./build'
          }
        }
      }
    }

    stage('Run acceptance tests') {
      steps {
        script {
          INFRAPOOL_EXECUTORV2_AGENT_0.agentSh './test_acceptance'
          INFRAPOOL_EXECUTORV2_AGENT_0.agentStash name: 'acceptance-output', includes: 'output/acceptance/*.xml'
        }
      }
      post {
        always {
          unstash 'acceptance-output'
          junit 'output/acceptance/*.xml'
        }
      }
    }

    stage('Validate installation script') {
      parallel {
        stage('Validate installation on Ubuntu 20:04') {
          steps {
            script {
              INFRAPOOL_EXECUTORV2_AGENT_0.agentSh 'bin/installer-test --ubuntu-20.04'
            }
          }
        }
        stage('Validate installation on Ubuntu 18:04') {
          steps {
            script {
              INFRAPOOL_EXECUTORV2_AGENT_0.agentSh 'bin/installer-test --ubuntu-18.04'
            }
          }
        }
        stage('Validate installation on Ubuntu 16:04') {
          steps {
            script {
              INFRAPOOL_EXECUTORV2_AGENT_0.agentSh 'bin/installer-test --ubuntu-16.04'
            }
          }
        }
      }
    }
  }

  post {
    always {
      script {
        releaseInfraPoolAgent(".infrapool/release_agents")
      }
    }
  }
}
