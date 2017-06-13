#!/usr/bin/env groovy

pipeline {
  agent { label 'executor-v2' }

  options {
    timestamps()
    buildDiscarder(logRotator(numToKeepStr: '30'))
  }

  stages {
    stage('Test: Unit') {
      steps {
        sh './test.sh'
        junit 'junit.xml'
      }
    }
    stage('Build') {
      steps {
        sh './build.sh'
      }
    }
    stage('Test: Acceptance') {
      steps {
        sh 'cp ./pkg/linux-amd64/summon .'
        dir('acceptance') {
          sh 'make'
        }
        // TODO: collect the acceptance test results
      }
    }
    stage('Package') {
      steps {
        sh 'sudo chmod -R 777 pkg/'  // TODO: remove need to sudo here
        sh './package.sh'
        archiveArtifacts artifacts: 'pkg/**/*', fingerprint: true
      }
    }
  }

  post {
    failure {
      slackSend(color: 'danger', message: "${env.JOB_NAME} #${env.BUILD_NUMBER} FAILURE (<${env.BUILD_URL}|Open>)")
    }
    unstable {
      slackSend(color: 'warning', message: "${env.JOB_NAME} #${env.BUILD_NUMBER} UNSTABLE (<${env.BUILD_URL}|Open>)")
    }
  }
}
