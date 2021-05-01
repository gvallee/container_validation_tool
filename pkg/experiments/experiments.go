// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, NVIDIA CORPORATION. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package experiments

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gvallee/go_exec/pkg/advexec"
	"github.com/gvallee/go_exec/pkg/results"
	"github.com/gvallee/go_hpc_jobmgr/pkg/app"
	"github.com/gvallee/go_hpc_jobmgr/pkg/implem"
	"github.com/gvallee/go_hpc_jobmgr/pkg/jm"
	"github.com/gvallee/go_hpc_jobmgr/pkg/mpi"
	"github.com/gvallee/go_util/pkg/util"
	"github.com/sylabs/singularity-mpi/pkg/buildenv"
	"github.com/sylabs/singularity-mpi/pkg/builder"
	"github.com/sylabs/singularity-mpi/pkg/container"
	"github.com/sylabs/singularity-mpi/pkg/launcher"
	"github.com/sylabs/singularity-mpi/pkg/sy"
	"github.com/sylabs/singularity-mpi/pkg/sys"
)

// ContainerConfig is a structure that represents the configuration of an experiment
type ContainerConfig struct {
	// HostMPI gathers all the data about the MPI to use on the host
	HostMPI implem.Info

	// ContainerMPI gathers all the data about the MPI to use in the container
	ContainerMPI implem.Info

	// Container gathers all the data about the container
	Container container.Config

	// HostBuildEnv gathers all the data about the environment to use to build the software for the host
	HostBuildEnv buildenv.Info

	// ContainerBuildEnv gathers all the data about the environment to use to build the software for the container
	ContainerBuildEnv buildenv.Info

	// App gathers all the data about the application to include in the container
	App app.Info

	// Result gathers all the data related to the result of an experiment
	Result results.Result
}

// GetImplemFromExperiments returns the MPI implementation that is associated
// to the experiments
func GetMPIImplemFromExperiments(experiments []ContainerConfig) (*implem.Info, error) {
	// Fair assumption: all experiments are based on the same MPI
	// implementation (we actually check for that and the implementation
	// is only included in the experiment structure so that the structure
	// is self-contained).
	if len(experiments) == 0 {
		return nil, fmt.Errorf("no experiment")
	}

	return &experiments[0].HostMPI, nil
}

func createNewContainer(myContainerMPICfg *mpi.Config, exp ContainerConfig, sysCfg *sys.Config, syConfig *sy.MPIToolConfig) advexec.Result {
	var res advexec.Result

	/* CREATE THE CONTAINER MPI CONFIGURATION */
	if sysCfg.Persistent != "" && util.FileExists(myContainerMPICfg.Container.Path) {
		log.Printf("* %s already exists, skipping...\n", myContainerMPICfg.Container.Path)
		return res
	}

	/* CREATE THE MPI CONTAINER */

	res = createMPIContainer(&exp.App, myContainerMPICfg, &exp.ContainerBuildEnv, sysCfg)
	if res.Err != nil {
		err := launcher.SaveErrorDetails(&exp.HostMPI, &myContainerMPICfg.Implem, sysCfg, &res)
		if err != nil {
			res.Err = fmt.Errorf("failed to save error details: %s", err)
			return res
		}
		res.Err = fmt.Errorf("failed to create container: %s", res.Err)
		return res
	}

	return res
}

func setExperimentCfg(exp ContainerConfig, sysCfg *sys.Config, syConfig *sy.MPIToolConfig) (mpi.Config, mpi.Config, error) {
	var myHostMPICfg mpi.Config
	var myContainerMPICfg mpi.Config

	myHostMPICfg.Buildenv = exp.HostBuildEnv
	myHostMPICfg.Implem = exp.HostMPI

	if !util.PathExists(myHostMPICfg.Buildenv.BuildDir) {
		err := os.MkdirAll(myHostMPICfg.Buildenv.BuildDir, 0755)
		if err != nil {
			return myHostMPICfg, myContainerMPICfg, fmt.Errorf("failed to create %s: %s", myHostMPICfg.Buildenv.BuildDir, err)
		}
	} else {
		log.Printf("Build directory on host already exists: %s", myHostMPICfg.Buildenv.BuildDir)
	}
	if !util.PathExists(myHostMPICfg.Buildenv.ScratchDir) {
		err := os.MkdirAll(myHostMPICfg.Buildenv.ScratchDir, 0755)
		if err != nil {
			return myHostMPICfg, myContainerMPICfg, fmt.Errorf("failed to create %s: %s", myHostMPICfg.Buildenv.ScratchDir, err)
		}
	} else {
		log.Printf("Build directory on host already exists: %s", myHostMPICfg.Buildenv.ScratchDir)
	}

	myContainerMPICfg.Implem = exp.ContainerMPI
	myContainerMPICfg.Buildenv = exp.ContainerBuildEnv
	myContainerMPICfg.Container.Name = container.GetContainerDefaultName(exp.Container.Distro, exp.ContainerMPI.ID, exp.ContainerMPI.Version, exp.App.Name, container.HybridModel) + ".sif"
	myContainerMPICfg.Container.Path = filepath.Join(myContainerMPICfg.Buildenv.InstallDir, myContainerMPICfg.Container.Name)
	exp.Container.Path = myContainerMPICfg.Container.Path
	myContainerMPICfg.Container.Model = container.HybridModel
	myContainerMPICfg.Container.URL = sy.GetImageURL(&myContainerMPICfg.Implem, sysCfg)
	myContainerMPICfg.Container.BuildDir = myContainerMPICfg.Buildenv.BuildDir
	myContainerMPICfg.Container.InstallDir = myContainerMPICfg.Buildenv.InstallDir
	myContainerMPICfg.Container.Distro = exp.Container.Distro

	if !util.PathExists(myContainerMPICfg.Buildenv.BuildDir) {
		err := os.MkdirAll(myContainerMPICfg.Buildenv.BuildDir, 0755)
		if err != nil {
			return myHostMPICfg, myContainerMPICfg, fmt.Errorf("failed to create %s: %s", myContainerMPICfg.Buildenv.BuildDir, err)
		}
	} else {
		log.Printf("Build directory on host already exists: %s", myContainerMPICfg.Buildenv.BuildDir)
	}
	if !util.PathExists(myContainerMPICfg.Buildenv.ScratchDir) {
		err := os.MkdirAll(myContainerMPICfg.Buildenv.ScratchDir, 0755)
		if err != nil {
			return myHostMPICfg, myContainerMPICfg, fmt.Errorf("failed to create %s: %s", myContainerMPICfg.Buildenv.ScratchDir, err)
		}
	} else {
		log.Printf("Build directory on host already exists: %s", myContainerMPICfg.Buildenv.ScratchDir)
	}

	log.Println("* Host MPI Configuration *")
	log.Println("-> Building MPI in", myHostMPICfg.Buildenv.BuildDir)
	log.Println("-> Installing MPI in", myHostMPICfg.Buildenv.InstallDir)
	log.Println("-> MPI implementation:", myHostMPICfg.Implem.ID)
	log.Println("-> MPI version:", myHostMPICfg.Implem.Version)
	log.Println("-> MPI URL:", myHostMPICfg.Implem.URL)

	log.Println("* Container MPI configuration *")
	log.Println("-> Build container in", exp.ContainerBuildEnv.BuildDir)
	log.Println("-> Target Linux distribution in container:", exp.Container.Distro)
	log.Println("-> Storing container in", exp.ContainerBuildEnv.InstallDir)
	log.Println("-> Container full path: ", exp.Container.Path)
	log.Println("-> MPI implementation:", myContainerMPICfg.Implem.ID)
	log.Println("-> MPI version:", myContainerMPICfg.Implem.Version)
	log.Println("-> MPI URL:", myContainerMPICfg.Implem.URL)

	return myHostMPICfg, myContainerMPICfg, nil
}

// Run configure, install and execute a given experiment
func Run(exp ContainerConfig, sysCfg *sys.Config, syConfig *sy.MPIToolConfig) (bool, results.Result, advexec.Result) {
	var expRes results.Result
	var execRes advexec.Result

	/* Figure out details about the experiment's configuration */
	myHostMPICfg, myContainerMPICfg, err := setExperimentCfg(exp, sysCfg, syConfig)
	if err != nil {
		execRes.Err = fmt.Errorf("failed to set experiment's configuration: %s", err)
		expRes.Pass = false
		return false, expRes, execRes
	}
	jobmgr := jm.Detect()
	b, err := builder.Load(&myHostMPICfg.Implem)
	if err != nil {
		execRes.Err = fmt.Errorf("unable to load a builder: %s", err)
		expRes.Pass = false
		return false, expRes, execRes
	}

	/* Capture the hardware/system configuration in order to capture provence of the experiment */
	//SOMETHING

	/* Install MPI on the host */
	execRes = b.InstallOnHost(&myHostMPICfg.Implem, &myHostMPICfg.Buildenv, sysCfg)
	if execRes.Err != nil {
		execRes.Err = fmt.Errorf("failed to install MPI on host")
		err = launcher.SaveErrorDetails(&exp.HostMPI, &myContainerMPICfg.Implem, sysCfg, &execRes)
		if err != nil {
			execRes.Err = fmt.Errorf("failed to save error details: %s", err)
		}
		expRes.Pass = false
		return false, expRes, execRes
	}
	if !sys.IsPersistent(sysCfg) {
		defer func() {
			execRes = b.UninstallHost(&myHostMPICfg.Implem, &myHostMPICfg.Buildenv, sysCfg)
			if execRes.Err != nil {
				log.Fatalf("failed to uninstall MPI: %s", err)
			}
		}()
	}

	/* Prepare the container image */
	if syConfig.BuildPrivilege || sysCfg.Nopriv {
		if !util.PathExists(exp.Container.Path) {
			execRes = createNewContainer(&myContainerMPICfg, exp, sysCfg, syConfig)
			if execRes.Err != nil {
				execRes.Err = fmt.Errorf("failed to create container: %s", err)
				expRes.Pass = false
				return false, expRes, execRes
			}
		} else {
			log.Printf("%s already exists, skipping build\n", exp.Container.Path)
		}
	} else {
		err = container.PullContainerImage(&myContainerMPICfg.Container, &myContainerMPICfg.Implem, sysCfg, syConfig)
		if err != nil {
			execRes.Err = fmt.Errorf("failed to pull container: %s", err)
			expRes.Pass = false
			return false, expRes, execRes
		}
	}

	/* Prepare the command to run the actual experiment */
	log.Println("* Running Test(s)...")

	expRes, execRes = launcher.Run(&exp.App, &myHostMPICfg, &exp.HostBuildEnv, &myContainerMPICfg, &jobmgr, sysCfg, nil)
	if !expRes.Pass {
		return false, expRes, execRes
	}
	if execRes.Err != nil {
		execRes.Err = fmt.Errorf("failed to run experiment: %s", execRes.Err)
		err = launcher.SaveErrorDetails(&exp.HostMPI, &myContainerMPICfg.Implem, sysCfg, &execRes)
		if err != nil {
			execRes.Err = fmt.Errorf("failed to save error details: %s", err)
		}
		expRes.Pass = false
		return false, expRes, execRes
	}

	log.Printf("* Successful run - Analysing data...")

	err = processOutput(&execRes, &expRes, &exp.App, sysCfg)
	if err != nil {
		execRes.Err = fmt.Errorf("failed to process output: %s", err)
		expRes.Pass = false
		return false, expRes, execRes
	}

	log.Println("-> Experiment successfully executed")
	log.Printf("* Experiment's note: %s", expRes.Note)

	expRes.Pass = true
	return false, expRes, execRes
}

// GetOutputFilename returns the name of the file that is associated to the experiments
// to run
func GetOutputFilename(mpiImplem string, sysCfg *sys.Config) error {
	sysCfg.OutputFile = mpiImplem + "-init-results.txt"

	if sysCfg.NetPipe {
		sysCfg.OutputFile = mpiImplem + "-netpipe-results.txt"
	}

	if sysCfg.IMB {
		sysCfg.OutputFile = mpiImplem + "-imb-results.txt"
	}

	return nil
}

// createMPIContainer creates a container based on a specific configuration.
func createMPIContainer(appInfo *app.Info, mpiCfg *mpi.Config, env *buildenv.Info, sysCfg *sys.Config) advexec.Result {
	var res advexec.Result
	var b builder.Builder

	containerCfg := &mpiCfg.Container

	b, res.Err = builder.Load(&mpiCfg.Implem)
	if res.Err != nil {
		return res
	}

	log.Println("Creating MPI container...")
	res.Err = b.GenerateDeffile(appInfo, &mpiCfg.Implem, env, containerCfg, sysCfg)
	if res.Err != nil {
		res.Stderr = fmt.Sprintf("failed to generate Singularity definition file: %s", res.Err)
		log.Printf("%s\n", res.Stderr)
		return res
	}

	res.Err = container.Create(&mpiCfg.Container, sysCfg)
	if res.Err != nil {
		res.Stderr = fmt.Sprintf("failed to create container image: %s", res.Err)
		log.Printf("%s\n", res.Stderr)
		return res
	}

	return res
}

// Pruning removes the experiments for which we already have results
func Pruning(experiments []ContainerConfig, existingResults []results.Result) []ContainerConfig {
	// No optimization at the moment, double loop and creation of a new array
	var experimentsToRun []ContainerConfig
	//	for j := 0; j < len(experiments); j++ {
	for _, experiment := range experiments {
		found := false
		for _, result := range existingResults {
			if experiment.HostMPI.Version == result.HostMPI.Version && experiment.ContainerMPI.Version == result.ContainerMPI.Version {
				log.Printf("We already have results for %s on the host and %s in a container, skipping...\n", experiment.HostMPI.Version, experiment.ContainerMPI.Version)
				found = true
				break
			}
		}
		if !found {
			// No associated results
			experimentsToRun = append(experimentsToRun, experiment)
		}
	}

	return experimentsToRun
}
