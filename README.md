# idea_dotenv

Converts JetBrains IDE run configurations (`*.run.xml`) into dotenv files.

## Installation

`go install github.com/GeekchanskiY/idea_dotenv@latest`

## Usage

`idea_dotenv [-o dir] path/to/.runConfigurations`

Each `name.run.xml` that defines environment variables is written to `.name.env`. The default output directory is the parent of the input directory. `-o` writes somewhere else. Existing files are replaced. New files are created with mode `0600`.

Values that contain spaces, quotes, `#`, `$`, or newlines are quoted. IDE macros such as `$PROJECT_DIR$` are copied through unchanged.

A `*.run.xml` with several `<configuration>` entries becomes one dotenv file per configuration, using each configuration's `name` attribute. Configurations with no `<envs>` are skipped.

## License

MIT. See [LICENSE.md](LICENSE.md).
