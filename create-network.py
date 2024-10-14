print("enter the number of storage nodes")
n_sn=int(input())
file = open("docker-network.yaml","w")
file.write("version: '3'\n\n")
file.write("networks:\n")
file.write("  iotblck:\n")
file.write("    ipam:\n")
file.write("      config:\n")
file.write("        - subnet: 150.0.0.0/24\n")
file.write("\n")
file.write("services:\n")
file.write("  discovery:\n")
file.write("    image: disc:1.0\n")
file.write("    networks:\n")
file.write("      iotblck:\n")
file.write("        ipv4_address: 150.0.0.100\n")
file.write("\n")
for i in range(n_sn):
    file.write("  storage"+str(i)+":\n")
    file.write("    image: sto:1.0\n")
    file.write("    networks:\n")
    file.write("      - iotblck\n")
    file.write("    depends_on:\n")
    file.write("      - discovery\n")
    if i > 0:
        file.write("      - storage"+str(i-1)+"\n")

print("enter the number of gateway nodes")
n_gw=int(input())
file.write("  gateway0"+":\n")
file.write("    image: gw:1.0\n")
file.write("    networks:\n")
file.write("      - iotblck\n")
file.write("    depends_on:\n")
file.write("      - discovery\n")
file.write("      - storage"+str(n_sn-1)+"\n")
for i in range(1,n_gw):
    file.write("  gateway"+str(i)+":\n")
    file.write("    image: gw:1.0\n")
    file.write("    networks:\n")
    file.write("      - iotblck\n")
    file.write("    depends_on:\n")
    file.write("      - discovery\n")
    file.write("      - gateway"+str(i-1)+"\n")

file1 = open("outputs/output.sh","w")
file1.write("docker logs -f iotbc_discovery_1 > "+"discovery.txt\n")
for i in range(n_sn):
    file1.write("docker logs -f iotbc_storage"+str(i)+"_1 > sn"+str(i)+".txt\n")
for i in range(n_gw):
    file1.write("docker logs -f iotbc_gateway"+str(i)+"_1 > gw"+str(i)+".txt\n")