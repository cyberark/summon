#!/usr/bin/env groovy

pipeline {
  agent { label 'executor-v2' }

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

    stage('Validate Changelog') {
      steps { sh './bin/parse-changelog' }
    }

    stage('Run unit tests') {
      steps {
        sh './test_unit'
        sh 'mv output/c.out .'
        ccCoverage("gocov", "--prefix github.com/cyberark/summon")
      }
      post {
        always {
          junit 'output/junit.xml'
          cobertura autoUpdateHealth: false, autoUpdateStability: false, coberturaReportFile: 'output/coverage.xml', conditionalCoverageTargets: '100, 0, 0', failUnhealthy: true, failUnstable: false, lineCoverageTargets: '74, 0, 0', maxNumberOfBuilds: 0, methodCoverageTargets: '92, 0, 0', onlyStable: false, sourceEncoding: 'ASCII', zoomCoverageChart: false
        }
      }
    }

    stage('Build Release Artifacts') {
      when {
        not {
          tag "v*"
        }
      }

      steps {
        sh './build --snapshot'
        archiveArtifacts 'dist/goreleaser/'
      }
    }

    stage('Build Release Artifacts and Create Pre Release') {
      // Only run this stage when triggered by a tag
      when { tag "v*" }

      steps {
        dir('./pristine-checkout') {
          // Go releaser requires a pristine checkout
          checkout scm

          // Create draft release
          sh "summon --yaml 'GITHUB_TOKEN: !var github/users/conjur-jenkins/api-token' ./build"
          archiveArtifacts 'dist/goreleaser/'
        }
      }
    }

    stage('Run acceptance tests') {
      steps {
        sh './test_acceptance'
      }
      post {
        always {
          junit 'output/acceptance/*.xml'
        }
      }
    }

    stage('Validate installation script') {
      parallel {
        stage('Validate installation on Ubuntu 20:04') {
          steps {
            sh 'bin/installer-test --ubuntu-20.04'
          }
        }
        stage('Validate installation on Ubuntu 18:04') {
          steps {
            sh 'bin/installer-test --ubuntu-18.04'
          }
        }
        stage('Validate installation on Ubuntu 16:04') {
          steps {
            sh 'bin/installer-test --ubuntu-16.04'
          }
        }
      }
    }
  }

  post {
    always {
      cleanupAndNotify(currentBuild.currentResult)
    }
  }
}
