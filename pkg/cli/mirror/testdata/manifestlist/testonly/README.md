# NOTICE

This folder contains all of the information needed to create a dummy OCI layout in the directory called "layout".
This OCI layout can only be used for test case purposes. The images inside the generated manifest list are built 
off of scratch in order to minimize its size. While these images contain a declarative config, these are NOT 
deployable catalogs since they do not contain real executables nor do they contain any libraries that 
would normally be necessary to support an executable. In other words, these image WILL NOT RUN.