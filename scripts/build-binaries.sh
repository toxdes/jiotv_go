#!/usr/bin/env bash

arm64_only=false
for arg in "$@"; do
    case "$arg" in
        --arm64-only|--arm-only) arm64_only=true ;;
    esac
done

if $arm64_only; then
    echo "Building arm64 binary only..."
    mkdir -p bin
    fork_suffix=$(cat "$(dirname "$0")/fork-suffix" 2>/dev/null || echo "")
    ldflags="-s -w"
    [[ -n "$fork_suffix" ]] && ldflags+=" -X main.versionSuffix=$fork_suffix"
    CGO_ENABLED=0 GOEXPERIMENT=jsonv2,greenteagc GOOS=linux GOARCH=arm64 go build -o "bin/jiotv_go-linux-arm64" -trimpath -ldflags="$ldflags" .
    echo "Done: bin/jiotv_go-linux-arm64"
    exit 0
fi

mkdir -p bin
allowed_archs="amd64 arm arm64 386 riscv64"
echo "Building binaries for allowed architectures: $allowed_archs"
for var in $(go tool dist list); do
    IFS='/' read -r os arch <<< "$var"
    # skip disallowed archs
    if [[ ! " $allowed_archs " =~ " $arch " ]]; then
        echo "Skipping: $var"
        continue
    fi
    
    file_name="jiotv_go-${os}-${arch}"
    case "$os" in
        "windows" | "linux" | "darwin")
            output_name="bin/${file_name}"
            if [[ "$os" == "windows" ]]; then
                output_name+=".exe"
            fi
            echo "Building $var"
            fork_suffix=$(cat "$(dirname "$0")/fork-suffix" 2>/dev/null || echo "")
            ldflags="-s -w"
            [[ -n "$fork_suffix" ]] && ldflags+=" -X main.versionSuffix=$fork_suffix"
            CGO_ENABLED=0 GOEXPERIMENT=jsonv2,greenteagc GOOS="$os" GOARCH="$arch" go build -o "${output_name}" -trimpath -ldflags="$ldflags" .
        ;;
        "android")
            echo "Building $var"
            cc=""
            cxx=""
            case "$arch" in
                "arm")
                    cc="armv7a-linux-androideabi28-clang"
                    cxx="armv7a-linux-androideabi28-clang++"
                    ;;
                "arm64")
                    cc="aarch64-linux-android32-clang"
                    cxx="aarch64-linux-android32-clang++"
                    ;;
                "amd64")
                    cc="x86_64-linux-android32-clang"
                    cxx="x86_64-linux-android32-clang++"
                    ;;
                "386")
                    cc="i686-linux-android32-clang"
                    cxx="i686-linux-android32-clang++"
                    ;;    
                *)
                    echo "Skipping: $var"
                    continue
                    ;;
            esac
            fork_suffix=$(cat "$(dirname "$0")/fork-suffix" 2>/dev/null || echo "")
            ldflags="-s -w"
            [[ -n "$fork_suffix" ]] && ldflags+=" -X main.versionSuffix=$fork_suffix"
            CGO_ENABLED=1 GOEXPERIMENT=jsonv2,greenteagc GOOS="$os" GOARCH="$arch" CC="$cc" CXX="$cxx" go build -o "bin/${file_name}" -trimpath -ldflags="$ldflags" .
        ;;
        *)
            echo "Skipping: $var"
        ;;
    esac
done

# Build for android5 arm with CC=armv7a-linux-androideabi21-clang
echo "Building android5 arm"
fork_suffix=$(cat "$(dirname "$0")/fork-suffix" 2>/dev/null || echo "")
ldflags="-s -w"
[[ -n "$fork_suffix" ]] && ldflags+=" -X main.versionSuffix=$fork_suffix"
CGO_ENABLED=1 GOEXPERIMENT=jsonv2,greenteagc GOOS=android GOARCH=arm GOARM=7 CC="armv7a-linux-androideabi21-clang" CXX="armv7a-linux-androideabi21-clang++" go build -o bin/jiotv_go-android5-armv7 -trimpath -ldflags="$ldflags" .
