package moosefs

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	reg = regexp.MustCompile("\\s+")
)

type (
	EchoFunc func([]string) <-chan string
	PipeFunc func(<-chan string) <-chan string
)

func echo(lines []string) <-chan string {
	out := make(chan string)
	go func() {
		for i := range lines {
			out <- lines[i]
		}
		close(out)
	}()
	return out
}

func pipeline(source []string, echo EchoFunc, pipes ...PipeFunc) <-chan string {
	ch := echo(source)
	go func() {
		for i := range pipes {
			ch = pipes[i](ch)
		}
	}()
	return ch
}

func split(r rune) bool {
	return r == '#' || r == ':' || r == '|'
}

// scs
// show connected chunk servers
func scs(line string) string {
	info := strings.FieldsFunc(line, split)
	k := reg.ReplaceAllString(info[0], "_")
	// chunk servers:|10.206.16.3|9422|2|-|3.0.116|0|maintenance_off|0|377221120|10726932480|-|0|0|0
	ret := fmt.Sprintf("moosefs_%s_load{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[6])
	if strings.Contains(info[7], "maintenance_off") {
		ret += fmt.Sprintf("moosefs_%s_maintenance{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} 0\n",
			k, info[1], info[2], info[3], info[4], info[5])
	} else {
		ret += fmt.Sprintf("moosefs_%s_maintenance{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} 1\n",
			k, info[1], info[2], info[3], info[4], info[5])
	}
	ret += fmt.Sprintf("moosefs_%s_regular_hdd_space_chunks{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[8])
	ret += fmt.Sprintf("moosefs_%s_regular_hdd_space_used{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[9])
	ret += fmt.Sprintf("moosefs_%s_regular_hdd_space_total{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[10])

	ret += fmt.Sprintf("moosefs_%s_removal_hdd_space_chunks{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[12])
	ret += fmt.Sprintf("moosefs_%s_removal_hdd_space_used{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[13])
	ret += fmt.Sprintf("moosefs_%s_removal_hdd_space_total{ip=\"%s\",port=\"%s\",id=\"%s\",labels=\"%s\",version=\"%s\"} %s\n",
		k, info[1], info[2], info[3], info[4], info[5], info[14])
	return ret
}

// sig
// show only general master (leader) info
func sig(line string) string {
	info := strings.FieldsFunc(line, split)

	k := fmt.Sprintf("%s_%s", info[0], info[1])
	k = reg.ReplaceAllString(k, "_")
	k = strings.ReplaceAll(strings.ReplaceAll(k, "(", ""), ")", "")
	k = strings.ToLower(k)
	ret := fmt.Sprintf("moose_%s %s", k, info[2])
	return ret
}

// sim
// show only masters states
func sim(line string) string {
	// metadata servers:|10.206.0.16|3.0.116|-|1672207842.932502|6|not available|214470656|all:0.1529132% sys:0.1017077% user:0.0512055%|1672207200|0.033|Saved in background|FFCCFD8ADEFBE2DE
	info := strings.FieldsFunc(line, split)
	l := len(info)
	k := reg.ReplaceAllString(info[0], "_")
	ret := fmt.Sprintf(`moosefs_%s_metadata_version{ip="%s", version="%s"} %s\n`, k, info[1], info[2], strings.ReplaceAll(info[5], " ", ""))
	ret += fmt.Sprintf(`moosefs_%s_ram_used{ip="%s", version="%s"} %s\n`, k, info[1], info[2], info[7])
	ret += fmt.Sprintf(`moosefs_%s_cpu_used_all{ip="%s", version="%s"} %s\n`, k, info[1], info[2], strings.Split(info[9], " ")[0])
	ret += fmt.Sprintf(`moosefs_%s_cpu_used_sys{ip="%s", version="%s"} %s\n`, k, info[1], info[2], strings.Split(info[10], " ")[0])
	ret += fmt.Sprintf(`moosefs_%s_cpu_used_user{ip="%s", version="%s"} %s\n`, k, info[1], info[2], strings.Split(info[11], " ")[0])
	ret += fmt.Sprintf(`moosefs_%s_last_meta_save{ip="%s", version="%s", cksum="%s"} %s\n`, k, info[1], info[2], info[l-1], info[12])
	ret += fmt.Sprintf(`moosefs_%s_last_save_duration{ip="%s", version="%s", cksum="%s"} %s\n`, k, info[1], info[2], info[l-1], info[13])

	if strings.Contains(info[14], "aved in background") {
		ret += fmt.Sprintf(`moosefs_%s_last_saved_in_background{ip="%s", version="%s", cksum="%s"} 1\n`, k, info[1], info[2], info[l-1])
	} else {
		ret += fmt.Sprintf(`moosefs_%s_last_saved_in_background{ip="%s", version="%s", cksum="%s"} 0\n`, k, info[1], info[2], info[l-1])
	}
	return ret
}

// smu
// show only master memory usage
func smu(line string) string {
	// memory usage detailed info:|chunk hash|0|0
	info := strings.FieldsFunc(line, split)
	k := reg.ReplaceAllString(info[0], "_")
	ret := fmt.Sprintf("moosefs_%s_%s_used %s\n", k, info[1], info[2])
	ret += fmt.Sprintf("moosefs_%s_%s_allocated %s\n", k, info[1], info[3])
	return ret
}

// sic
// show only chunks info (goal/copies matrices)
func sic(line string) string {
	info := strings.FieldsFunc(line, split)
	ret := fmt.Sprintf("moosefs_%s %s", fmt.Sprintf("%s_%s", reg.ReplaceAllString(info[0], "_"),
		reg.ReplaceAllString(info[1], "_")), info[2])
	return ret
}

// ssc
// show storage classes
func ssc(line string) string {

	info := strings.FieldsFunc(line, split)
	k := reg.ReplaceAllString(info[0], "_")
	C := reg.ReplaceAllString(info[15], "")
	K := reg.ReplaceAllString(info[18], "")
	A := reg.ReplaceAllString(info[21], "")
	ret := fmt.Sprintf("moosefs_%s_create{id=\"%s\",name=\"%s\",mode=\"%s\",can=\"%s\",labels=\"%s\"} %s\n", k,
		info[1], info[2], info[4], info[13], C, info[14])
	ret += fmt.Sprintf("moosefs_%s_keep{id=\"%s\",name=\"%s\",mode=\"%s\",can=\"%s\",labels=\"%s\"} %s\n", k,
		info[1], info[2], info[4], info[16], K, info[17])
	ret += fmt.Sprintf("moosefs_%s_archive{id=\"%s\",name=\"%s\",mode=\"%s\",can=\"%s\",labels=\"%s\"} %s\n", k,
		info[1], info[2], info[4], info[19], A, info[20])

	overTotal := info[9]
	if overTotal == "-" {
		overTotal = "0"
	}
	underTotal := info[7]
	if underTotal == "-" {
		underTotal = "0"
	}
	archiveUnderTotal := info[10]
	if archiveUnderTotal == "-" {
		archiveUnderTotal = "0"
	}
	archiveExactTotal := info[11]
	if archiveExactTotal == "-" {
		archiveExactTotal = "0"
	}
	archiveOverTotal := info[12]
	if archiveOverTotal == "-" {
		archiveOverTotal = "0"
	}

	ret += fmt.Sprintf("moosefs_%s_files_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1], info[2], info[5])
	ret += fmt.Sprintf("moosefs_%s_dirs_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1], info[2], info[6])
	ret += fmt.Sprintf("moosefs_%s_standard_under_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], underTotal)
	ret += fmt.Sprintf("moosefs_%s_standard_exact_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], info[8])
	ret += fmt.Sprintf("moosefs_%s_standard_over_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], overTotal)
	ret += fmt.Sprintf("moosefs_%s_archived_under_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], archiveUnderTotal)
	ret += fmt.Sprintf("moosefs_%s_archived_exact_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], archiveExactTotal)
	ret += fmt.Sprintf("moosefs_%s_archived_over_total{id=\"%s\",name=\"%s\"} %s\n", k, info[1],
		info[2], archiveOverTotal)

	return ret
}

// show quota info
func squ(line string) string {
	info := strings.FieldsFunc(line, split)
	// k=gensub(/\s/, "_", "g", $1);  # keyword
	keyword := reg.ReplaceAllString(info[0], "_")
	// P=gensub(/\s/, "",  "g", $3);  # path
	path := reg.ReplaceAllString(info[2], "")
	// SI=gensub(/\s/, "", "g", $7);  # soft inodes
	softInodes := reg.ReplaceAllString(info[6], "")
	// SL=gensub(/\s/, "", "g", $8);  # soft length
	softLen := reg.ReplaceAllString(info[7], "")
	// SS=gensub(/\s/, "", "g", $9);  # soft size
	softSize := reg.ReplaceAllString(info[8], "")
	// SR=gensub(/\s/, "", "g", $10);  # soft real size
	softRealSize := reg.ReplaceAllString(info[9], "")
	// HI=gensub(/\s/, "", "g", $11); # hard inodes
	hardInodes := reg.ReplaceAllString(info[10], "")
	// HL=gensub(/\s/, "", "g", $12); # hard length
	hardLen := reg.ReplaceAllString(info[11], "")
	// HS=gensub(/\s/, "", "g", $13); # hard size
	hardSize := reg.ReplaceAllString(info[12], "")
	// HR=gensub(/\s/, "", "g", $14); # hard real size
	hardRealSize := reg.ReplaceAllString(info[13], "")
	// CI=gensub(/\s/, "", "g", $15); # current inodes
	currentInodes := reg.ReplaceAllString(info[14], "")
	// CL=gensub(/\s/, "", "g", $16); # current length
	currentLen := reg.ReplaceAllString(info[15], "")
	// CS=gensub(/\s/, "", "g", $17); # current size
	currentSize := reg.ReplaceAllString(info[16], "")
	// CR=gensub(/\s/, "", "g", $18); # current real size
	currentRealSize := reg.ReplaceAllString(info[17], "")

	ret := fmt.Sprintf("moosefs_%s_soft_inodes{path=\"%s\"} %s\n", keyword, path, softInodes)
	ret += fmt.Sprintf("moosefs_%s_soft_length{path=\"%s\"} %s\n", keyword, path, softLen)
	ret += fmt.Sprintf("moosefs_%s_soft_size{path=\"%s\"} %s\n", keyword, path, softSize)
	ret += fmt.Sprintf("moosefs_%s_soft_real{path=\"%s\"} %s\n", keyword, path, softRealSize)

	ret += fmt.Sprintf("moosefs_%s_hard_inodes{path=\"%s\"} %s\n", keyword, path, hardInodes)
	ret += fmt.Sprintf("moosefs_%s_hard_length{path=\"%s\"} %s\n", keyword, path, hardLen)
	ret += fmt.Sprintf("moosefs_%s_hard_size{path=\"%s\"} %s\n", keyword, path, hardSize)
	ret += fmt.Sprintf("moosefs_%s_hard_real{path=\"%s\"} %s\n", keyword, path, hardRealSize)

	ret += fmt.Sprint("moosefs_%s_current_inodes{path=\"%s\"} %s\n", keyword, path, currentInodes)
	ret += fmt.Sprint("moosefs_%s_current_length{path=\"%s\"} %s\n", keyword, path, currentLen)
	ret += fmt.Sprint("moosefs_%s_current_size{path=\"%s\"} %s\n", keyword, path, currentSize)
	ret += fmt.Sprint("moosefs_%s_current_real{path=\"%s\"} %s\n", keyword, path, currentRealSize)

	return ret
}

func Parse() {
	lines := make([]string, 0, 100)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if reg.ReplaceAllString(line, "") == "" {
			continue
		}
		lines = append(lines, line)
	}

	for _, line := range lines {
		switch {
		case strings.Contains(line, "active quotas:"):
			fmt.Printf("%s\n", squ(line))
		case strings.Contains(line, "metadata servers:"):
			fmt.Printf("%s\n", sim(line))
		case strings.Contains(line, "memory usage detailed info:"):
			fmt.Printf("%s\n", smu(line)) //
		case strings.Contains(line, "chunk servers:"):
			fmt.Printf("%s\n", scs(line))
		case strings.Contains(line, "master info:"):
			fmt.Printf("%s\n", sig(line))
		case strings.Contains(line, "chunkclass "):
			fmt.Printf("%s\n", sic(line)) //
		case strings.Contains(line, "storage classes:"):
			fmt.Printf("%s\n", ssc(line))
		}
	}
}
