import re
from sys import argv

# leftMostWord: poolMinBaseIdx=188 startBaseIdx=188

# [INFO]  text.go:2055 moveWord ###1: text.go:1523: baseIdx=47 45: "Educator" base=191.89 {342.48 375.46 600.11 609.11}
reGo= re.compile(r'moveWord ###1:\s+text\.go:\d+:\s+baseIdx=\d+\s+(\d+):\s+"(.+)"\s+base=(\S+)')
lineGo = 'moveWord ###1: text.go:1523: baseIdx=47 45: "Educator" base=191.89 {342.48 375.46 600.11 609.11}'
m = reGo.search(lineGo)
assert m

#  addWord ###1: TextOutputDev.cc:3080   21:`Current` base=213.85 x=53.98..82.19
reCpp= re.compile(r'addWord ###1:\s+TextOutputDev.\cc:\d+\s+(\d+):\s*`(.+)`\s+base=(\S+)')
lineCpp = 'addWord ###1: TextOutputDev.cc:3080   21:`Current` base=213.85 x=53.98..82.19'
m = reCpp.search(lineCpp)
assert m
def scan(path, regex):
	n = 0
	nLines = 0
	matches = []
	with open(path, 'rt') as f:
		for line in f:
			nLines += 1
			m = regex.search(line)
			if not m:
				continue
			n += 1
			idx = int(m.group(1))
			text = m.group(2)
			base = float(m.group(3))
			# print('%d: %4d %4d' % (n, pool, start))
			matches.append((nLines, idx,text,base, line[:-1]))
	return nLines, matches

_, matches1 = scan(argv[1], reGo)
_, matches2 = scan(argv[2], reCpp)
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-30s    %s'% (argv[1], argv[2]))
badIds = []
map1 = {}
map2 = {}
for i in range(n):
	n1, i1, t1, b1, l1 = matches1[i]
	n2, i2, t2, b2, l2 = matches2[i]
	bad = i1 != i2 or t1 != t2 or b1 != b2
	if bad:
		badIds.append(i)
	# (479 Mathematician; 696.84) 
	# part1 = '(%3d %8s %5.2f)' % (i1, t1, b1)
	# part2 = '(%3d %8s %5.2f)' % (i2, t2, b2)
	part1 = '(%5.2f %s)' % (b1, t1)
	part2 = '(%5.2f %s)' % (b2, t2)
	marker = '******' if bad else ''
	print('%3d: %-30s %-30s %s' % (i, part1, part2, marker))
	map1[i] = part1
	map2[i] = part2
	# if bad:
	# 	print('%6d: %s' % (n1, l1))
	# 	print('%6d: %s' % (n2, l2))
	# if badI < 0:
	# 	if bad:
	# 		badI = i
	# elif i > badI + 5:
	# 	break
print('-' * 80)
print('%d bad of %d' % (len(badIds), n))
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))
print('bad=%d %s' % (len(badIds), badIds)) 

pam1 = {v: k for k, v in map1.items()}
pam2 = {v: k for k, v in map2.items()}
print('-' * 80)
edges = {}
for i in badIds:
	k1 = map1[i]
	# k2 = map2[i]
	j1 = pam2[k1]
	v1 = map2[j1]
	# v2 = pam1[k1]
	# print(type(i), type(k1), type(j1), type(v1))
	print('%20s %3d -> %3d' % (v1, i, j1))
	edges[i] = j1
	assert v1 == k1

cycles = []
print('-' * 80)
for i in badIds:
	j = i
	cycle = [j]
	while True:
		j = edges[j]
		if j == i:
			break
		cycle.append(j)
	cycles.append(cycle)
	print('%20s %3d -> %s' % ( map1[i], i, cycle))
