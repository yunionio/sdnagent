#!/bin/bash

set -o errexit
set -o pipefail

if [ "$DEBUG" == "true" ] ; then
    set -ex ;export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'
fi

readlink_mac() {
  cd `dirname $1`
  TARGET_FILE=`basename $1`

  # Iterate down a (possible) chain of symlinks
  while [ -L "$TARGET_FILE" ]
  do
    TARGET_FILE=`readlink $TARGET_FILE`
    cd `dirname $TARGET_FILE`
    TARGET_FILE=`basename $TARGET_FILE`
  done

  # Compute the canonicalized name by finding the physical path
  # for the directory we're in and appending the target file.
  PHYS_DIR=`pwd -P`
  REAL_PATH=$PHYS_DIR/$TARGET_FILE
}

get_current_arch() {
    local current_arch
    case $(uname -m) in
    x86_64)
        current_arch=amd64
        ;;
    aarch64)
        current_arch=arm64
        ;;
    riscv64)
        current_arch=riscv64
        ;;
    esac
    echo $current_arch
}

pushd $(cd "$(dirname "$0")"; pwd) > /dev/null
readlink_mac $(basename "$0")
cd "$(dirname "$REAL_PATH")"
CUR_DIR=$(pwd)
SRC_DIR=$(cd .. && pwd)
popd > /dev/null

DOCKER_DIR="$SRC_DIR/build/docker/"

REGISTRY=${REGISTRY:-docker.io/yunion}
TAG=${TAG:-latest}
CURRENT_ARCH=$(get_current_arch)
ARCH=${ARCH:-$CURRENT_ARCH}
PROJ=sdnagent
image_keyword=sdnagent

build_bin() {
    local BUILD_ARCH="$1";
    local BUILD_CC="$2";
    local BUILD_CGO="$3";

	docker run --rm \
		-v $SRC_DIR:/root/go/src/yunion.io/x/$PROJ \
        -v $SRC_DIR/_output/alpine-build:/root/go/src/yunion.io/x/$PROJ/_output \
		-v $SRC_DIR/_output/alpine-build/_cache:/root/.cache \
		registry.cn-beijing.aliyuncs.com/yunionio/alpine-build:3.22.2-go-1.24.9-0 \
		/bin/sh -c "set -ex;
            git config --global --add safe.directory /root/go/src/yunion.io/x/$PROJ;
            cd /root/go/src/yunion.io/x/$PROJ;
			$BUILD_ARCH $BUILD_CC $BUILD_CGO GOOS=linux make all;
			chown -R $(id -u):$(id -g) _output;
			find _output/bin -type f |xargs ls -lah"
}

buildx_and_push() {
    local tag=$1
    local file=$2
    local path=$3
    local arch=$4
    docker buildx build -t "$tag" --platform "linux/$arch" -f "$file" "$path" --push
    docker pull --platform "linux/$arch" "$tag"
}

get_image_name() {
    local component=$1
    local arch=$2
    local is_all_arch=$3
    local img_name="$REGISTRY/$component:$TAG"
    if [[ -n "$arch" ]]; then
        if [[ "$is_all_arch" == "true" || "$arch" != "CURRENT_ARCH" ]]; then
            img_name="${img_name}-$arch"
        fi
    fi
    echo $img_name
}

build_process_with_buildx() {
    local arch=$1
    local is_all_arch=$2
    local img_name=$(get_image_name $image_keyword $arch $is_all_arch)

    build_env="GOARCH=$arch "
	build_bin $build_env
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[$(readlink -f ${BASH_SOURCE}):${LINENO} ${FUNCNAME[0]}] return for DRY_RUN"
        return
    fi
	buildx_and_push $img_name $DOCKER_DIR/Dockerfile $SRC_DIR $arch
}

make_manifest_image() {
    local component=$1
    local img_name=$(get_image_name $component "" "false")
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[$(readlink -f ${BASH_SOURCE}):${LINENO} ${FUNCNAME[0]}] return for DRY_RUN"
        return
    fi
    docker buildx imagetools create -t $img_name \
        $img_name-amd64 \
        $img_name-arm64 \
        $img_name-riscv64
}

show_update_cmd() {
	local name="sdnagent"
	local spec="hostagent/SdnAgent"
    echo "kubectl patch oc -n onecloud default --type='json' -p='[{op: replace, path: /spec/${spec}/imageName, value: ${name}},{"op": "replace", "path": "/spec/${spec}/repository", "value": "${REGISTRY}"},{"op": "add", "path": "/spec/${spec}/tag", "value": "${TAG}"}]'"
}

cd $SRC_DIR

echo "Start to build for arch[$ARCH]"

case "$ARCH" in
    all)
        for arch in "arm64" "amd64" "riscv64"; do
            build_process_with_buildx $arch "true"
        done
        make_manifest_image $image_keyword
        ;;
    *)
        build_process_with_buildx $ARCH "false"
        ;;
esac

show_update_cmd
