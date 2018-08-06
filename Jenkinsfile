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
    stage('Build Go package') {
      steps {
        sh './build.sh'
        archiveArtifacts artifacts: "dist/*.tar.gz,dist/*.zip,dist/*.rb,dist/*.deb,dist/*.rpm,dist/*.txt", fingerprint: true
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
        sh 'cp ./dist/linux_amd64/summon summon'
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
  }

  post {
    always {
      cleanupAndNotify(currentBuild.currentResult)
    }
  }
}
