#!/usr/bin/env bash

#package=$1
#if [[ -z "$package" ]]; then
#  echo "usage: $0 <package-name>"
#  exit 1
#fi

package="cmd/discord-proxy/main.go"
package_split=(${package//\// })
package_name=${package_split[-1]}
package_name="discord-proxy"
output_dir="bin"

platforms=("linux/amd64" "windows/amd64" "darwin/amd64")

for platform in "${platforms[@]}"
do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    output_name=$package_name'-'$GOOS
    if [ $GOOS = "windows" ]; then
        output_name+='.exe'
    fi

    env GOOS=$GOOS GOARCH=$GOARCH go build -o "${output_dir}/${output_name}" $package
    if [ $? -ne 0 ]; then
        echo 'An error has occurred! Aborting the script execution...'
        exit 1
    fi
done
