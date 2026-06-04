pub mod mpp {
    pub mod contentpipeline {
        pub mod v1 {
            tonic::include_proto!("mpp.contentpipeline.v1");
        }
    }
}

pub const FILE_DESCRIPTOR_SET: &[u8] =
    tonic::include_file_descriptor_set!("content_pipeline_descriptor");
