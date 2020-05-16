from glob import glob
from sys import argv
from os import makedirs
from os.path import join, isdir, basename, exists


def cleanFile(path):
	lines = []
	with open(path, 'rt', errors='ignore') as f:
		for i1, line in enumerate(f):
			i = i1+1
			line = line[:-1].strip()
			if not line:
				continue
			# print("%d: %s" % (i, line))
			lines.append(line)
			# if i >= 100:
			# 	break
	return lines

def clean(inPath, outPath):
	lines = cleanFile(inPath)
	with open(outPath, 'wt') as f:
		f.writelines(['%s\n' % ln for ln in lines])

def cleanDir(inDir, outDir):
	assert isdir(inDir), inDir
	if exists(outDir):
		assert isdir(outDir),outDir
	assert inDir != outDir

	makedirs(outDir, exist_ok=True)
	for i, inPath in enumerate(glob(join(inDir, '*.txt'))):
		if isdir(inPath):
			continue
		name = basename(inPath)
		outPath = join(outDir, name)
		assert not isdir(outPath)
		print("%3d: %s -> %s" % (i, inPath, outPath))
		clean(inPath, outPath)


cleanDir(argv[1], argv[2])

