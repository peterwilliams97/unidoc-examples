import re
from sys import argv

# text.go: 1325 *@* %s *#* *
# !!1 rot = 0 - --------------
# !!2 baseIdx = 7 n = 2 - --------------
# 0 of 2: word: 31.26 {529.83 540.59 810.63 820.63} fontsize = 10.00 "VOX"
# 1 of 2: word: 31.26 {541.91 552.61 810.63 820.63} fontsize = 10.00 "POP"

# word   0: base = 31.26 {42.52 53.28 810.63 820.63} fontsize = 10.00 "VOX"

# textBlock: lines built blk=3--------------------------
reHeader = re.compile(r'\*@\*\s+(.*?)\s+\*#\*')
reRot = re.compile(r'!!1 rot\s*=\s*(\d)\s*--------------')
reBase = re.compile(r'!!2 baseIdx\s*=\s*(\d+)\s+n\s*=(\d+)\s*--------------')
reWord = re.compile(r'word\s+(\d+)\s*:\s*base=(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize=(\S+)\s*"(.*)"')

lineHeader = 'text.go:1325 *@* initial words *#*'
assert reHeader.search(lineHeader)
lineRot = '!!1 rot=0 ---------------'
assert reRot.search(lineRot)
lineBase = '!!2 baseIdx=7 n=2 ---------------'
assert reBase.search(lineBase)
lineWord = '"     word   0: base=110.62 {42.52 136.70 731.27 784.27} fontsize=53.00 "Print"'
assert reWord.search(lineWord)


def scan(path, verbose):
	instances = []
	header = None
	rots = []
	bases = []
	words = []
	lines = []
	state = 0
	done = False

	print('## scan "%s" verbose=%s' % (path, verbose))

	with open(path, 'rt', errors='ignore') as f:
		for i, line in enumerate(f):
			if done:
				break
			lines.append(line[:-1])
			line = line[:-1]
			for scan in (1, 2):
				if state == 0:
					if rots:
						instances.append((i, header, rots))
						header = None
						print('## "%s" Done instances=%d' % (path, len(instances)))
						done = True
						break
					m = reHeader.search(line)
					if m:
						state = 1
						header = m.group(1)
						rots = []
						break
				elif state == 1:
					if bases:
						rots.append((rot, bases))
						state = 0
						if verbose:
							print('## 1->0 rots=%d instance=%d' % (len(rots), len(instances)))
					m = reRot.search(line)
					if m:
						state = 2
						rot = int(m.group(1))
						bases = []
						break
				elif state == 2:
					if words:
						bases.append((baseIdx, words))
						state = 1
						if verbose:
							print('## 2->1 bases=%d rot=%d' % (len(bases), rot))
					m = reBase.search(line)
					if m:
						state = 3
						baseIdx = int(m.group(1))
						numWords = int(m.group(2))
						# print('$$$ base=%d numWords=%d' % (baseIdx, numWords))
						words = []
						break
				elif state == 3:
					m = reWord.search(line)
					if m:
						idx = int(m.group(1))
						base = float(m.group(2))
						llx = float(m.group(3))
						urx = float(m.group(4))
						lly = float(m.group(5))
						ury = float(m.group(6))
						fontsize = float(m.group(7))
						text = m.group(8)
						words.append((idx, base, llx, urx, lly, ury, fontsize, text, line))
						break
					else:
						assert len(words) == numWords, '%d!=%d >>%s<<' % (len(words),numWords,line)
						state = 2
						if verbose:
							print('## 3->2 words=%d baseIdx=%d' % (len(words), baseIdx))
			if state != 0:
				if verbose:
					print('\tstate=%d line=%d >>%s<<' % (state, i, line))
	assert instances
	return instances


TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL

exclusions = ['\x13','\x19']


def exclude(text):
	for e in exclusions:
		if e in text:
			return True
	return False

def compareInstance(instance1, instance2):
	i1, h1, rots1 = instance1
	i2, h2, rots2 = instance2
	print('rots1=%s %d' % (type(rots1), len(rots1)))
	print('rots1[0]=%s %d' % (type(rots1[0]), len(rots1[0])))
	rot1, bases1 = rots1[0]
	rot2, bases2 = rots2[0]
	print('rot1=%d rot2=%d' % (rot1, rot2))
	print('bases1=%d bases2=%d' % (len(bases1), len(bases2)))
	assert len(bases1) == len(bases2)
	n = min(len(bases1), len(bases2))

	for i in range(n):
		baseIdx1, words1 = bases1[i]
		baseIdx2, words2 = bases2[i]
		print('  bases %2d: baseIdx=%d words=%d | baseIdx=%d words=%d' % (
			i, baseIdx1, len(words1), baseIdx2, len(words2)))
		assert baseIdx1 == baseIdx2
		assert len(words1) == len(words2)

		m = len(words1)
		for j in range(m):
			w1 = words1[j]
			w2 = words2[j]
			idx1, base1, llx1, urx1, lly1, ury1, fontsize1, text1, line1 = w1
			idx2, base2, llx2, urx2, lly2, ury2, fontsize2, text2, line2 = w2
			msg = 'j=%d\n\tw1=>>%s<<\n\tw2=>>%s<<' % (j, line1, line2)
			assert base1 == base2, msg
			assert llx1 == llx2, msg
			assert urx1 == urx2, msg
			assert fontsize1 == fontsize2, msg
			# assert text1 not in exclusions, text1
			# assert text2 not in exclusions, text2
			if exclude(text1) or exclude(text2):
				print(msg)
			if exclude(text1) == exclude(text2):
				assert text1 == text2, msg
		# if baseIdx1 == 69:
		# 	print(msg)
		# 	break

verbose = False
instances1 = scan(argv[1], verbose)
instances2 = scan(argv[2], verbose)
print('%s %d instances %s' % (argv[1], len(instances1), [(i,h) for i,h,_ in instances1]))
print('%s %d instances %s' % (argv[2], len(instances2), [(i,h) for i,h,_ in instances2]))

compareInstance(instances1[0], instances2[0])
