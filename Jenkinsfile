#!/usr/bin/env groovy

pipeline {
  agent { label 'executor-v2' }

  options {
    timestamps()
    buildDiscarder(logRotator(numToKeepStr: '30'))
    skipDefaultCheckout()  // see 'post' below, once perms are fixed this is no longer needed
  }

  stages {
    stage('Checkout SCM') {
      steps {
        checkout scm
      }
    }
    stage('Build Go binaries') {
      steps {
        sh './build.sh linux:amd64'
        archiveArtifacts artifacts: 'output/summon-linux-amd64', fingerprint: true
      }
    }
    stage('Run unit tests') {
      steps {
        sh './test.sh'
        junit 'output/junit.xml'
      }
    }

    stage('Run acceptance tests') {
      steps {
        sh 'cp ./output/summon-linux-amd64 summon'
        dir('acceptance') {
          sh 'make'
        }
        // TODO: remove need to sudo here
        sh 'sudo chown -R jenkins:jenkins .'
        // TODO: collect the acceptance test results
      }
    }

    stage('Validate installation script') {
      parallel {
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

    stage('Package distribution tarballs') {
      steps {
        sh './build.sh'  // now build binaries for all distros
        sh './package.sh'
        archiveArtifacts artifacts: 'output/dist/*', fingerprint: true
      }
    }
  }

  post {
    always {
      cleanupAndNotify(currentBuild.currentResult)
    }
  }
}
