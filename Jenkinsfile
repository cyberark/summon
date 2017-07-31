#!/usr/bin/env groovy

pipeline {
  agent { label 'executor-v2' }

  options {
    timestamps()
    buildDiscarder(logRotator(numToKeepStr: '30'))
    skipDefaultCheckout()  // see 'Checkout SCM' below, once perms are fixed this is no longer needed
  }

  stages {
    stage('Checkout SCM') {
      steps {
        sh 'sudo chown -R jenkins:jenkins .'  // bad docker mount creates unreadable files TODO fix this

        checkout scm
      }
    }
    stage('Build Go binaries') {
      steps {
        sh './build.sh'
        archiveArtifacts artifacts: 'output/*', fingerprint: true
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
        // TODO: collect the acceptance test results
      }
    }
    stage('Package distribution tarballs') {
      steps {
        // sh 'sudo chmod -R 777 pkg/'  // TODO: remove need to sudo here
        sh './package.sh'
        // archiveArtifacts artifacts: 'pkg/**/*', fingerprint: true
      }
    }
  }

  post {
    always {
      sh 'sudo chown -R jenkins:jenkins .'
      deleteDir()
    }
    failure {
      slackSend(color: 'danger', message: "${env.JOB_NAME} #${env.BUILD_NUMBER} FAILURE (<${env.BUILD_URL}|Open>)")
    }
    unstable {
      slackSend(color: 'warning', message: "${env.JOB_NAME} #${env.BUILD_NUMBER} UNSTABLE (<${env.BUILD_URL}|Open>)")
    }
  }
}
