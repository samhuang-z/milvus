def name = ''

pipeline {
    options {
        skipDefaultCheckout true
    }
    agent {
        kubernetes {
            cloud '4am'
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
                        name = run_tekton_pipeline 'revision': "$env.BRANCH_NAME", 'arch':'arm64'
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
  taskRunTemplate:
    serviceAccountName:  robot-tekton
    podTemplate:
      tolerations:
      - key: "node-role.kubernetes.io/arm"
        operator: "Exists"
        effect: "NoSchedule"
      nodeSelector:
        "kubernetes.io/arch": "arm64"
        # hostNetwork: true
      securityContext:
        fsGroup: 65532
  workspaces:
  - name: cache-conan
    persistentVolumeClaim:
        claimName: conan-os-ubuntu20.04-arch-amd64-compiler-gcc9-buildtype-release
  - name: shared-data
    volumeClaimTemplate:
      spec:
        storageClassName: local-path
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
EOF
"""

    println input

    def ret = sh( label: "tekton pipeline run", script: input, returnStdout: true)

    name = ret.split('/')[1].split(' ')[0]
    return name
}
