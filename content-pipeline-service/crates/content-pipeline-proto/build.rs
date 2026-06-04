fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_file = "../../proto/mpp/contentpipeline/v1/content_pipeline.proto";
    let proto_include = "../../proto";
    let descriptor_path =
        std::path::PathBuf::from(std::env::var("OUT_DIR")?).join("content_pipeline_descriptor.bin");

    println!("cargo:rerun-if-changed={proto_file}");

    unsafe {
        std::env::set_var("PROTOC", protoc_bin_vendored::protoc_bin_path()?);
    }

    tonic_prost_build::configure()
        .file_descriptor_set_path(descriptor_path)
        .compile_protos(&[proto_file], &[proto_include])?;

    Ok(())
}
