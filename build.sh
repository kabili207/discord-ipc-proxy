#!/usr/bin/env bash

package="./cmd/discord-proxy"
package_name="discord-proxy"
output_dir="bin"

platforms=("linux/amd64" "windows/amd64" "darwin/amd64")

for platform in "${platforms[@]}"; do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    output_name="${package_name}-${GOOS}"
    if [ "$GOOS" = "windows" ]; then
        output_name+='.exe'
    fi

    echo "Building ${output_name}..."
    env GOOS=$GOOS GOARCH=$GOARCH go build -o "${output_dir}/${output_name}" "$package"
    if [ $? -ne 0 ]; then
        echo "Build failed for ${platform}"
        exit 1
    fi
done

echo "Done."
