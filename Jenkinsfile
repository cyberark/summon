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

    stage('Build Go package') {
      steps {
        sh './build'
        archiveArtifacts artifacts: "dist/*.tar.gz,dist/*.zip,dist/*.rb,dist/*.deb,dist/*.rpm,dist/*.txt", fingerprint: true
      }
    }

    stage('Run unit tests') {
      steps {
        sh './test_unit'
      }
      post {
        always {
          junit 'output/junit.xml'
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
        stage('Validate installation on Ubuntu 14:04') {
          steps {
            sh 'bin/installer-test --ubuntu-14.04'
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
