def name = ''

pipeline {
    options {
        skipDefaultCheckout true
    }
    agent {
        kubernetes {
            yaml '''
        spec:
          securityContext:
            runAsUser: 1000 # default UID of jenkins user in agent image
          containers:
          - name: kubectl
            image: bitnami/kubectl:1.27.14
            command:
            - cat
            tty: true
          - name: tkn
            image: milvusdb/krte:tkn-0.37.0
            command:
            - cat
            tty: true
        '''
        }
    }
    stages {
        stage('build') {
            steps {
                container('kubectl') {
                    script {
                        name = run_tekton_pipeline 'revision': "$env.BRANCH_NAME", 'arch':'amd64', 'computing_engine':'gpu', 'milvus_env_os_name':'ubuntu22.04'
                    }
                }

                container('tkn') {
                    script {
                        sh """
                            tkn pipelinerun logs ${name} -f -n milvus-ci
                            tkn pipelinerun describe ${name} -n milvus-ci
                        """
                    }
                }
            }
        }
    }
}





def run_tekton_pipeline(Map args) {
    def input = """
cat << EOF | kubectl create -f -
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: milvus-build-
  namespace: milvus-ci
spec:
  pipelineRef:
    name: milvus-clone-build-push
  taskRunSpecs:
    - pipelineTaskName: build-push
      serviceAccountName: robot-tekton
  workspaces:
  - name: shared-data
    volumeClaimTemplate:
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 100Gi
  params:
  - name: revision
    value: ${args.revision}
  - name: arch
    value: ${args.arch}
  - name: computing_engine
    value: ${args.computing_engine} 
  - name:  milvus_env_os_name 
    value: ${args.milvus_env_os_name}

EOF
"""

    println input

    def ret = sh( label: "tekton pipeline run", script: input, returnStdout: true)

    name = ret.split('/')[1].split(' ')[0]
    return name
}
