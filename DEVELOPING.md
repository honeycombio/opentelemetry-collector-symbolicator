# Developing

## mdatagen
We generate documentation via the [mdatagen](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/mdatagen#using-the-metadata-generator) tool. To regenerate documentation, you must have `mdatagen` installed.

To install:
1. Clone the opentelemetry repo (`git clone git@github.com:open-telemetry/opentelemetry-collector.git`)
2. Follow the instructions on the mdatagen readme (`cd cmd/mdatagen && go install .`)
3. If you are managing your go install with asdf, you may need to run `asdf reshim` in order for your shell to find the `mdatagen` binary
4. In this repo, cd to the package getting regenerated and run `mdatagen metadata.yaml`. 
