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
        sh 'package_name=(summon*linux-amd64); cp ./output/$package_name summon'
        dir('acceptance') {
          sh 'make'
        }
        // TODO: remove need to sudo here
        sh 'sudo chown -R jenkins:jenkins .'
        // TODO: collect the acceptance test results
      }
    }
    stage('Package distribution tarballs') {
      steps {
        sh './package.sh'
        archiveArtifacts artifacts: 'output/dist/*', fingerprint: true
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
